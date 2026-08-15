import { useEffect, useMemo, useRef, useState } from 'react'
import Hls from 'hls.js'
import {
  MediaPlayer,
  MediaProvider,
  Track,
  isHLSProvider,
  useMediaRemote,
  type MediaPlayerInstance,
  type MediaProviderAdapter,
  type VideoSrc,
} from '@vidstack/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  DefaultMenuRadioGroup,
  DefaultMenuSection,
  DefaultVideoLayout,
  defaultLayoutIcons,
} from '@vidstack/react/player/layouts/default'
import '@vidstack/react/player/styles/base.css'
import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import type { Video } from '../../api/videos'
import {
  coverUrl,
  fetchHistory,
  fetchVideoAudioTracks,
  fetchVideoSubtitles,
  hlsPlaylistUrl,
  putHistory,
  remuxUrl,
  streamUrl,
  subtitleUrl,
} from '../../api/videos'
import { getActiveTab, subscribeTabs } from '../../tabs/manager'
import type { PlayMode } from '../../lib/playability'

// Resume only kicks in beyond these boundaries so an almost-finished video
// starts fresh and a stray 1s glitch never seeks anywhere.
const RESUME_MIN = 10
const RESUME_TAIL = 20
// Progress persistence is throttled (ADR plan: ~10s cadence).
const SAVE_INTERVAL = 10

// VideoPlayer plays through the tier the caller selected (ADR-006, 2026-08):
//   direct  → the original file over HTTP Range (/api/stream/{id})
//   remux   → a container-only stream-copied MP4 over Range (/api/stream/{id}/remux)
//   transcode → on-demand HLS transcode via hls.js (/api/stream/{id}/hls/…)
// The caller decides the tier with playMode (lib/playability.ts); this player
// only renders it. Direct and remux are both plain MP4 Range streams — only the
// transcode tier wires the local hls.js into Vidstack's HLS provider.
export function VideoPlayer({
  video,
  mode = 'direct',
  onEnded,
}: {
  video: Video
  mode?: Exclude<PlayMode, 'none'>
  onEnded?: () => void
}) {
  const playerRef = useRef<MediaPlayerInstance>(null)
  const remote = useMediaRemote(playerRef)
  const queryClient = useQueryClient()
  const [resumeAt, setResumeAt] = useState(0)
  const posRef = useRef(0)
  const lastSaveRef = useRef(0)
  // Selected audio track (container stream index); switching re-loads the stream
  // with that track and restores the position (see onTrackSelect/onCanPlay).
  const [audio, setAudio] = useState(0)
  // pendingSeekRef carries the position to restore after a track-switch reload.
  // It is re-applied on every can-play/loaded-metadata/time-update until the
  // player actually catches up (a single can-play seek can be dropped before
  // the new stream is seekable, especially for HLS).
  const pendingSeekRef = useRef(0)
  const wasPlayingRef = useRef(false)
  const shouldResumeRef = useRef(false)

  // Every HLS play gets a fresh session token (per-video), so each viewer keeps
  // its own segment cache and concurrent terminals never share files mid-write
  // (ADR-002 多终端并发). The playlist emits segment URLs carrying this token.
  // A track switch gets a new token so the new track transcodes into its own
  // session instead of reusing the previous track's cached segments.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  const hlsSession = useMemo(() => (mode === 'transcode' ? crypto.randomUUID() : ''), [mode, audio])

  // Subtitle track menu: the backend enumerates every playable subtitle source
  // (sidecar file + embedded text tracks) and each becomes a <Track>, so the
  // player's built-in menu offers switching between them.
  const subtitles = useQuery({ queryKey: ['subtitles', video.id], queryFn: () => fetchVideoSubtitles(video.id) })
  // Only tracks the browser can actually render become <Track> elements: the
  // sidecar and embedded text tracks (extracted to WebVTT). Bitmap tracks
  // (PGS/VobSub, playable=false) cannot be converted to text and would 404.
  const subtitleTracks = subtitles.data?.subtitles?.filter((t) => t.playable !== false)

  // Audio track menu (multi-track containers): backend-enumerated tracks are
  // offered only on the dynamic tiers (remux/transcode) that can actually map a
  // specific track. Direct serves the raw file and uses the browser's default
  // track (per plan, native video.audioTracks switching is out of scope).
  const audioTracks = useQuery({
    queryKey: ['audio', video.id],
    queryFn: () => fetchVideoAudioTracks(video.id),
  })
  const audioList = mode === 'direct' ? [] : (audioTracks.data?.audio ?? [])

  useEffect(() => {
    lastSaveRef.current = 0
    fetchHistory(video.id)
      .then(({ history }) => {
        const dur = video.duration || 0
        if (history && history.progress > RESUME_MIN && (dur === 0 || history.progress < dur - RESUME_TAIL)) {
          setResumeAt(history.progress)
        }
      })
      .catch(() => {})
  }, [video.id, video.duration])

  useEffect(
    () => () => {
      // Save the resume position, then invalidate the detail page's history
      // query so it re-reads the fresh progress after leaving playback.
      const save = posRef.current > 0 ? putHistory(video.id, posRef.current).catch(() => {}) : Promise.resolve()
      void save.then(() => queryClient.invalidateQueries({ queryKey: ['history', video.id] }))
    },
    [video.id, queryClient],
  )

  // Auto-pause when the user switches away from the library tab: the player's
  // component tree stays alive (keep-alive), so the media would otherwise keep
  // playing in the background like an uncontrolled browser tab.
  useEffect(
    () =>
      subscribeTabs(() => {
        if (getActiveTab() !== 'library') {
          remote.pause()
        }
      }),
    [remote],
  )

  // Inject the local hls.js into Vidstack's HLS provider instead of letting it
  // fetch a build from a CDN; the transcode tier is served only when the direct
  // and remux tiers both failed canPlayType.
  function onProviderChange(provider: MediaProviderAdapter | null) {
    if (provider && isHLSProvider(provider)) {
      provider.library = Hls
    }
  }

  // /api/stream/{id} has no extension, so Vidstack cannot infer the media type
  // and would fall back to a HEAD probe (which can fail on cancelled requests).
  // Provide the type explicitly to select the right loader.
  const mediaSrc = useMemo<VideoSrc | { src: string; type: 'application/x-mpegurl' }>(() => {
    if (mode === 'transcode') {
      return { src: hlsPlaylistUrl(video.id, hlsSession, audio), type: 'application/x-mpegurl' }
    }
    return { src: mode === 'remux' ? remuxUrl(video.id, audio) : streamUrl(video.id), type: 'video/mp4' }
  }, [mode, video.id, hlsSession, audio])

  function onTimeUpdate(detail: { currentTime: number }) {
    posRef.current = detail.currentTime
    const dur = video.duration || 0
    if (dur > 0 && detail.currentTime >= dur - 1) {
      return
    }
    if (detail.currentTime - lastSaveRef.current >= SAVE_INTERVAL) {
      lastSaveRef.current = detail.currentTime
      void putHistory(video.id, detail.currentTime).catch(() => {})
    }
  }

  // onTrackSelect switches the playback audio track: the stream re-loads with
  // the new ?audio= index (remux) / new HLS session (transcode), and the current
  // position (and play state) is restored once the new stream can play.
  function onTrackSelect(index: number) {
    if (index === audio) return
    pendingSeekRef.current = posRef.current
    shouldResumeRef.current = wasPlayingRef.current
    setAudio(index)
  }

  // ensurePendingSeek re-applies the restore-after-switch seek until the player
  // catches up. A single onCanPlay seek can be dropped before the new stream is
  // seekable (HLS especially), so it is re-issued on every playback event and
  // only cleared once currentTime reaches the target, then resumes if the user
  // was playing before the switch.
  function ensurePendingSeek() {
    const target = pendingSeekRef.current
    if (target <= 0) return
    const t = playerRef.current?.currentTime ?? 0
    if (t < target - 1) {
      remote.seek(target)
    } else {
      pendingSeekRef.current = 0
      if (shouldResumeRef.current) {
        shouldResumeRef.current = false
        remote.play()
      }
    }
  }

  const currentAudioLabel = audioList.find((t) => t.index === audio)?.label

  return (
    <div className="relative h-full w-full">
      <MediaPlayer
        ref={playerRef}
        src={mediaSrc}
        poster={coverUrl(video.id)}
        title={video.title}
        playsInline
        className="h-full w-full bg-black"
        style={{ aspectRatio: 'auto' }}
        onProviderChange={onProviderChange}
        onCanPlay={() => {
          ensurePendingSeek()
          if (resumeAt > 0) {
            remote.seek(resumeAt)
            setResumeAt(0)
          }
        }}
        onLoadedMetadata={ensurePendingSeek}
        onTimeUpdate={(detail) => {
          ensurePendingSeek()
          onTimeUpdate(detail)
        }}
        onPlaying={() => {
          wasPlayingRef.current = true
        }}
        onPause={() => {
          wasPlayingRef.current = false
        }}
        onEnded={() => {
          posRef.current = 0
          void putHistory(video.id, 0).catch(() => {})
          onEnded?.()
        }}
      >
        <MediaProvider>
          {subtitleTracks && subtitleTracks.length > 0 ? (
            subtitleTracks.map((t) => (
              <Track
                key={t.kind === 'embedded' ? `e${t.index}` : 'sidecar'}
                src={subtitleUrl(video.id, t.kind === 'embedded' ? t.index : undefined)}
                kind="subtitles"
                label={t.label || '字幕'}
              />
            ))
          ) : !subtitleTracks ? (
            <Track src={subtitleUrl(video.id)} kind="subtitles" label="字幕" />
          ) : null}
        </MediaProvider>
        <DefaultVideoLayout
          icons={defaultLayoutIcons}
          slots={{
            settingsMenuItemsEnd:
              audioList.length > 1 ? (
                <DefaultMenuSection label="音轨" value={currentAudioLabel ?? `音轨 ${audio + 1}`}>
                  <DefaultMenuRadioGroup
                    value={String(audio)}
                    options={audioList.map((t) => ({
                      label: t.channels ? `${t.label} · ${t.channels} 声道` : t.label,
                      value: String(t.index),
                    }))}
                    onChange={(v) => onTrackSelect(Number(v))}
                  />
                </DefaultMenuSection>
              ) : null,
          }}
        />
      </MediaPlayer>
    </div>
  )
}
