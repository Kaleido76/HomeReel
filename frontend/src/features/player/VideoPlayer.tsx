import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Hls from 'hls.js'
import { useQuery, useQueryClient } from '@tanstack/react-query'
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
  fetchVideoPrefs,
  fetchVideoSubtitles,
  hlsPlaylistUrl,
  putHistory,
  putVideoPrefs,
  remuxUrl,
  streamUrl,
  subtitleUrl,
  type PlaybackPrefsPatch,
} from '../../api/videos'
import { getActiveTab, subscribeTabs } from '../../tabs/manager'
import { NEAR_END, RESUME_MIN, RESUME_TAIL, SAVE_INTERVAL } from '../../lib/playback'
import type { PlayMode } from '../../lib/playability'

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

  // Playback selection cache (ADR-006 player prefs): the remembered audio track,
  // subtitle source and volume. Remembered values are auto-applied once per
  // stream and refreshed only when the user manually changes a selection — the
  // save handlers below are all suppressed while applyPrefs runs.
  const prefs = useQuery({ queryKey: ['prefs', video.id], queryFn: () => fetchVideoPrefs(video.id) })
  // applyingPrefsRef guards the auto-apply's remote calls (changeVolume/mute/
  // changeTextTrackMode all dispatch request events that the save handlers would
  // otherwise treat as a user choice).
  const applyingPrefsRef = useRef(false)
  // Per-field application state keyed by the current mediaSrc: switching audio
  // tracks changes mediaSrc and resets audio/subtitle so the new stream gets its
  // remembered selections again; volume persists across src changes on the same
  // element and is applied exactly once.
  const appliedPrefsRef = useRef<{
    src: VideoSrc | { src: string; type: 'application/x-mpegurl' } | null
    audio: boolean
    subtitle: boolean
    volume: boolean
  }>({ src: null, audio: false, subtitle: false, volume: false })
  // Volume is recorded only on a real user adjustment, never on auto-apply; the
  // unmount save is the safety net for a change made right before leaving.
  const userChangedVolumeRef = useRef(false)
  const lastVolumeSaveRef = useRef(0)
  // The provider only accepts volume/text-track requests once media metadata has
  // loaded (set in onLoadedMetadata); gating keeps a pre-ready auto-apply from
  // dropping its remote call while still consuming the per-field flag.
  const playerReadyRef = useRef(false)

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
  const subtitleTracks = useMemo(
    () => subtitles.data?.subtitles?.filter((t) => t.playable !== false),
    [subtitles.data],
  )

  // Audio track menu (multi-track containers): backend-enumerated tracks are
  // offered only on the dynamic tiers (remux/transcode) that can actually map a
  // specific track. Direct serves the raw file and uses the browser's default
  // track (per plan, native video.audioTracks switching is out of scope).
  const audioTracks = useQuery({
    queryKey: ['audio', video.id],
    queryFn: () => fetchVideoAudioTracks(video.id),
  })
  const audioList = mode === 'direct' ? [] : (audioTracks.data?.audio ?? [])

  // savePrefs writes a selection the user manually made. For a series member the
  // write is series-scoped (shared across every episode by track name), so the
  // caller passes NAMES and the query cache is re-read afterwards (the effective
  // record may switch scope when the first series choice is recorded). Re-entry
  // into playback within the same page session therefore re-fetches fresh data.
  const savePrefs = useCallback(
    (patch: PlaybackPrefsPatch) => {
      void putVideoPrefs(video.id, patch)
        .then(() => {
          queryClient.invalidateQueries({ queryKey: ['prefs', video.id] })
          const seriesId = prefs.data?.series_id
          if (seriesId) queryClient.invalidateQueries({ queryKey: ['series-prefs', seriesId] })
        })
        .catch(() => {})
    },
    [video.id, queryClient, prefs.data?.series_id],
  )

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
      // query so it re-reads the fresh progress after leaving playback. A
      // series member also refreshes its series detail (series progress card /
      // member rows aggregate per-member progress there).
      const save = posRef.current > 0 ? putHistory(video.id, posRef.current).catch(() => {}) : Promise.resolve()
      void save.then(() => {
        queryClient.invalidateQueries({ queryKey: ['history', video.id] })
        if (video.show_id) queryClient.invalidateQueries({ queryKey: ['series'] })
      })
      // Flush a volume change made right before leaving (the 1s live throttle
      // may have skipped it). Only a real user adjustment is saved, never an
      // auto-applied remembered value.
      if (userChangedVolumeRef.current) {
        const el = playerRef.current
        if (el && typeof el.volume === 'number') {
          savePrefs({ volume: el.volume, muted: el.muted })
        }
      }
    },
    [video.id, video.show_id, queryClient, savePrefs],
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
    if (dur > 0 && detail.currentTime >= dur - NEAR_END) {
      return
    }
    if (detail.currentTime - lastSaveRef.current >= SAVE_INTERVAL) {
      lastSaveRef.current = detail.currentTime
      void putHistory(video.id, detail.currentTime).catch(() => {})
    }
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

  // applyPrefs auto-applies the remembered selections (audio track on the
  // dynamic tiers, subtitle source including explicit off, volume+muted). A
  // series-scoped row is resolved by matching the remembered track NAME against
  // this episode's own track list (ADR-006 player prefs 修订): a name with no
  // matching track in the current episode is left unapplied, a video-scoped row
  // applies its concrete per-video values. It is driven by an effect over its
  // inputs plus onLoadedMetadata, and applies each field exactly once per stream
  // via appliedPrefsRef — fields whose data has not arrived yet are retried on
  // the next pass, never skipped.
  const applyPrefs = useCallback(() => {
    const applied = appliedPrefsRef.current
    if (applied.src !== mediaSrc) {
      applied.src = mediaSrc
      applied.audio = false
      applied.subtitle = false
      // volume is a player-level property that survives a src change; once is
      // enough, re-applying it would re-dispatch volume-change for nothing.
    }
    const p = prefs.data?.prefs
    if (!p || !playerRef.current) return
    applyingPrefsRef.current = true
    try {
      if (!applied.audio && mode !== 'direct') {
        const list = audioTracks.data?.audio
        if (list) {
          applied.audio = true
          if (p.scope === 'series') {
            if (typeof p.audio_track_name === 'string') {
              const idx = list.findIndex((t) => t.label === p.audio_track_name)
              if (idx >= 0) setAudio(idx)
            }
          } else if (typeof p.audio_track === 'number' && p.audio_track >= 0 && p.audio_track < list.length) {
            setAudio(p.audio_track)
          }
        }
      }
      if (!applied.subtitle && playerReadyRef.current && subtitleTracks !== undefined) {
        applied.subtitle = true
        const tracks = playerRef.current.textTracks
        if (tracks) {
          let targetId: string | undefined
          if (p.scope === 'series') {
            if (typeof p.subtitle_name === 'string') {
              if (p.subtitle_name === '') {
                targetId = ''
              } else {
                const sub = subtitleTracks.find((t) => t.label === p.subtitle_name)
                targetId = sub ? (sub.kind === 'embedded' ? `e${sub.index}` : 'sidecar') : undefined
              }
            }
          } else if (typeof p.subtitle_id === 'string') {
            targetId = p.subtitle_id
          }
          if (targetId === '') {
            remote.disableCaptions()
          } else if (targetId) {
            const idx = tracks.toArray().findIndex((t) => t.id === targetId)
            if (idx >= 0) remote.changeTextTrackMode(idx, 'showing')
          }
        }
      }
      if (!applied.volume && playerReadyRef.current && (typeof p.volume === 'number' || typeof p.muted === 'boolean')) {
        applied.volume = true
        if (typeof p.volume === 'number') remote.changeVolume(p.volume)
        if (typeof p.muted === 'boolean') {
          if (p.muted) remote.mute()
          else remote.unmute()
        }
      }
    } finally {
      applyingPrefsRef.current = false
    }
  }, [prefs.data, mediaSrc, mode, audioTracks.data, subtitleTracks, remote])

  useEffect(() => {
    applyPrefs()
  }, [applyPrefs])

  // onTrackSelect switches the playback audio track: the stream re-loads with
  // the new ?audio= index (remux) / new HLS session (transcode), and the current
  // position (and play state) is restored once the new stream can play. A manual
  // track choice is also remembered — by NAME on a series member so the choice
  // follows every episode.
  function onTrackSelect(index: number) {
    if (index === audio) return
    pendingSeekRef.current = posRef.current
    shouldResumeRef.current = wasPlayingRef.current
    setAudio(index)
    if (prefs.data?.series_id) {
      const label = audioList.find((t) => t.index === index)?.label
      if (label) savePrefs({ audio_track_name: label })
    } else {
      savePrefs({ audio_track: index })
    }
  }

  // onSubtitleTrackRequest fires on every text-track-change-request — the
  // player's built-in captions menu (and caption button) is the only source,
  // so any such event is a genuine user choice. The chosen track is resolved
  // back to its id ("sidecar" / "e<N>", matching the <Track> ids below) and
  // remembered; an explicit off stores an empty string. On a series member the
  // remembered value is the track's NAME so the choice follows every episode.
  function onSubtitleTrackRequest(detail: { index: number; mode: string }) {
    if (applyingPrefsRef.current) return
    const tracks = playerRef.current?.textTracks
    if (!tracks) return
    if (detail.mode === 'disabled' || detail.index < 0) {
      savePrefs(prefs.data?.series_id ? { subtitle_name: '' } : { subtitle_id: '' })
      return
    }
    const track = tracks.toArray()[detail.index]
    if (!track?.id) return
    if (prefs.data?.series_id) {
      const sub = subtitleTracks?.find((t) => (t.kind === 'embedded' ? `e${t.index}` : 'sidecar') === track.id)
      if (sub?.label) savePrefs({ subtitle_name: sub.label })
    } else {
      savePrefs({ subtitle_id: track.id })
    }
  }

  // onVolumeChange persists a manual volume/mute adjustment. Auto-applied volume
  // is suppressed by applyingPrefsRef; a 1s throttle keeps slider drags from
  // spamming the API, and the unmount handler below flushes the final value.
  function onVolumeChange(detail: { volume: number; muted: boolean }) {
    if (applyingPrefsRef.current) return
    userChangedVolumeRef.current = true
    const now = Date.now()
    if (now - lastVolumeSaveRef.current < 1000) return
    lastVolumeSaveRef.current = now
    savePrefs({ volume: detail.volume, muted: detail.muted })
  }

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
        onLoadedMetadata={() => {
          playerReadyRef.current = true
          ensurePendingSeek()
          applyPrefs()
        }}
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
        onVolumeChange={onVolumeChange}
        onMediaTextTrackChangeRequest={onSubtitleTrackRequest}
      >
        <MediaProvider>
          {subtitleTracks && subtitleTracks.length > 0 ? (
            subtitleTracks.map((t) => (
              <Track
                key={t.kind === 'embedded' ? `e${t.index}` : 'sidecar'}
                id={t.kind === 'embedded' ? `e${t.index}` : 'sidecar'}
                src={subtitleUrl(video.id, t.kind === 'embedded' ? t.index : undefined)}
                kind="subtitles"
                label={t.label || '字幕'}
              />
            ))
          ) : !subtitleTracks ? (
            <Track id="sidecar" src={subtitleUrl(video.id)} kind="subtitles" label="字幕" />
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
