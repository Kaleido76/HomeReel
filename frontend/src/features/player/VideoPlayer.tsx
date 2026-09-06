import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import Hls from 'hls.js'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  MediaPlayer,
  MediaProvider,
  Track,
  isHLSProvider,
  useMediaRemote,
  useMediaState,
  type MediaPlayerInstance,
  type MediaProviderAdapter,
  type VideoSrc,
} from '@vidstack/react'
import { TheatreModeExitIcon, TheatreModeIcon } from '@vidstack/react/icons'
import {
  DefaultMenuRadioGroup,
  DefaultMenuSection,
  DefaultTooltip,
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
  type VideoPrefsResponse,
} from '../../api/videos'
import { type SeriesPlaybackPrefs } from '../../api/series'
import { getActiveTab, subscribeTabs } from '../../tabs/manager'
import { NEAR_END, RESUME_MIN, RESUME_TAIL, SAVE_INTERVAL } from '../../lib/playback'
import type { PlayMode } from '../../lib/playability'
import { useWebFullscreen } from '../../lib/useWebFullscreen'

// StreamSrc is the media source handed to <MediaPlayer>: a plain MP4 (direct /
// remux) or the transcode HLS playlist. /api/stream/{id} has no extension, so the
// type must be explicit (Vidstack cannot infer it from the URL).
type StreamSrc = VideoSrc | { src: string; type: 'application/x-mpegurl' }

// FullscreenClock shows the current system time (HH:mm) in the player's top-right
// corner while the media is fullscreen (native or the web fullscreen overlay). It
// sits inside <MediaPlayer> so it can read the fullscreen state; the clock is
// pointer-events-none so it never blocks the controls underneath.
function FullscreenClock({ webActive }: { webActive: boolean }) {
  const fullscreen = useMediaState('fullscreen') || webActive
  const [time, setTime] = useState(() => new Date())

  useEffect(() => {
    if (!fullscreen) return
    const id = window.setInterval(() => setTime(new Date()), 30_000)
    return () => window.clearInterval(id)
  }, [fullscreen])

  if (!fullscreen) return null
  const hh = String(time.getHours()).padStart(2, '0')
  const mm = String(time.getMinutes()).padStart(2, '0')
  return (
    <div className="pointer-events-none absolute right-2 top-2 z-[60] rounded bg-black/40 px-2 py-0.5 text-base font-semibold leading-tight text-white/90">
      {hh}:{mm}
    </div>
  )
}

// FullscreenToolbarButton is the shared chrome for the two fullscreen controls
// (web + native). It must activate like the layout's own buttons, which fire on
// a primary pointer-up rather than click: the layout tooltip cancels touchstart,
// and on touch devices that suppresses the synthetic click a plain onClick
// would rely on (ADR-006). The pointer-up covers mouse/touch/pen and the
// keyboard click (detail === 0) covers Enter/Space; the trailing mouse click
// (detail >= 1) after a handled pointer-up is ignored so it does not fire twice.
function FullscreenToolbarButton({
  active,
  onToggle,
  enterText,
  exitText,
  icon,
}: {
  active: boolean
  onToggle: () => void
  enterText: string
  exitText: string
  icon: ReactNode
}) {
  return (
    <DefaultTooltip content={active ? exitText : enterText} placement="top end">
      <button
        type="button"
        aria-label={active ? exitText : enterText}
        data-media-tooltip="fullscreen"
        data-active={active ? '' : undefined}
        className="vds-fullscreen-button vds-button"
        onPointerUp={(e) => {
          if (e.button === 0) onToggle()
        }}
        onClick={(e) => {
          if (e.detail === 0) onToggle()
        }}
      >
        {icon}
      </button>
    </DefaultTooltip>
  )
}

// PlayerWebFullscreenButton is the dedicated "web fullscreen" control: it fills
// the browser viewport with the CSS overlay (useWebFullscreen) instead of the
// native Fullscreen API, so the browser chrome stays visible and the app's own
// controls/subtitles are never hidden behind a native overlay. It is the only
// fullscreen control on mobile (the native button is dropped there) and a
// distinct "fill the viewport" option on desktop, behaving identically on every
// device (ADR-006). The theatre-mode glyph (YouTube-style "fill the viewport"
// without true fullscreen) tells it apart from the native fullscreen button,
// which keeps the standard fullscreen-arrow icon.
function PlayerWebFullscreenButton({
  active,
  onToggle,
}: {
  active: boolean
  onToggle: () => void
}) {
  return (
    <FullscreenToolbarButton
      active={active}
      onToggle={onToggle}
      enterText="网页全屏"
      exitText="退出网页全屏"
      icon={
        active ? (
          <TheatreModeExitIcon className="vds-icon" />
        ) : (
          <TheatreModeIcon className="vds-icon" />
        )
      }
    />
  )
}

// PlayerNativeFullscreenButton is the layout's native fullscreen control: it
// requests real browser fullscreen via the Fullscreen API. It is shown only on
// desktop; on mobile it is hidden because the web fullscreen button already
// covers the CSS overlay path. It reuses the default button's classes and icon
// set so it looks identical to the original control (ADR-006).
function PlayerNativeFullscreenButton({ onToggle }: { onToggle: () => void }) {
  const fullscreen = useMediaState('fullscreen')
  const Icon = fullscreen
    ? defaultLayoutIcons.FullscreenButton.Exit
    : defaultLayoutIcons.FullscreenButton.Enter
  return (
    <FullscreenToolbarButton
      active={fullscreen}
      onToggle={onToggle}
      enterText="全屏"
      exitText="退出全屏"
      icon={<Icon className="vds-icon" />}
    />
  )
}

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
  // Web fullscreen is the dedicated CSS overlay control on every device
  // (ADR-006); native fullscreen remains a separate desktop-only control. The
  // hook's mobile flag only decides which control the layout shows (mobile drops
  // the native button), it never changes how web fullscreen behaves.
  const webFullscreen = useWebFullscreen()

  // Web (CSS) fullscreen and native fullscreen are two independent controls now
  // (see the slots below): the web fullscreen button always toggles the CSS
  // overlay on every device, and the native fullscreen button requests real
  // browser fullscreen on desktop. Both are kept mutually exclusive.
  const toggleWebFullscreen = useCallback(() => {
    // If the browser is already in native fullscreen, back out of it first so
    // the CSS overlay is not trapped under the browser chrome.
    remote.exitFullscreen()
    webFullscreen.toggle()
  }, [remote, webFullscreen])

  const toggleNativeFullscreen = useCallback(() => {
    remote.toggleFullscreen()
  }, [remote])

  // Restore the native "f" shortcut that the replaced default button used to
  // own. It maps to native fullscreen (the standard desktop behavior); the web
  // fullscreen button is reachable on screen. Only handled when the player (or
  // its descendant control) has focus so typing an "f" elsewhere is unaffected.
  useEffect(() => {
    const el = playerRef.current?.el
    if (!el) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'f' || e.key === 'F') {
        const t = e.target as Node | null
        if (el.contains(t)) {
          e.preventDefault()
          toggleNativeFullscreen()
        }
      }
    }
    el.addEventListener('keydown', onKeyDown)
    return () => el.removeEventListener('keydown', onKeyDown)
  }, [toggleNativeFullscreen])
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
  const prefs = useQuery({
    queryKey: ['prefs', video.id],
    queryFn: () => fetchVideoPrefs(video.id),
    // Force a fresh read on every entry into playback: the global staleTime
    // (30s) would otherwise reuse a cached prefs row (e.g. an empty one), so a
    // remembered volume saved elsewhere would not be applied (Bug #2).
    refetchOnMount: 'always',
  })
  // applyingPrefsRef guards the auto-apply's remote calls (changeTextTrackMode
  // dispatches request events that the save handlers would otherwise treat as a
  // user choice). Volume is applied via the controlled volume/muted props, so it
  // is not routed through remote here.
  const applyingPrefsRef = useRef(false)
  // Per-field application state keyed by the current mediaSrc: switching audio
  // tracks changes mediaSrc and resets audio/subtitle so the new stream gets its
  // remembered selections again. Volume persists across src changes on the same
  // element and is handled separately via the controlled props.
  const appliedPrefsRef = useRef<{
    src: StreamSrc | null
    audio: boolean
    subtitle: boolean
  }>({ src: null, audio: false, subtitle: false })
  // Volume is recorded only on a real user adjustment, never on auto-apply; the
  // unmount save is the safety net for a change made right before leaving.
  const userChangedVolumeRef = useRef(false)
  // Debounce timer for the volume slider: during a drag the volume changes on
  // every pointer move, so we wait for it to settle (400ms with no change) and
  // then persist the final value — the value at press is not meaningful (Bug #4).
  const volumeTimerRef = useRef<number | undefined>(undefined)
  // Controlled volume/muted passed to <MediaPlayer> (Vidstack resets the media
  // volume to 100% on can-play unless a volume prop is given). These start
  // undefined (= Vidstack default 100%) and are adopted from the remembered prefs
  // once, then kept in sync with user changes via onVolumeChange (Bug #2).
  const [playerVolume, setPlayerVolume] = useState<number | undefined>(undefined)
  const [playerMuted, setPlayerMuted] = useState<boolean | undefined>(undefined)
  const volumeAdoptedRef = useRef(false)
  // The provider only accepts text-track requests once media metadata has loaded
  // (set in onLoadedMetadata); gating keeps a pre-ready auto-apply from dropping
  // its remote call while still consuming the per-field flag.
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
          const seriesId = prefs.data?.series_id
          // Update the local query cache immediately so the detail pages reflect
          // the new memory without waiting for a refetch (Bug #3). The volume is
          // stored in whatever row is effective, so we patch it in place.
          queryClient.setQueryData<VideoPrefsResponse>(['prefs', video.id], (old) => {
            if (!old) return old
            return { ...old, prefs: old.prefs ? { ...old.prefs, ...patch } : { scope: 'video', ...patch } }
          })
          if (seriesId) {
            queryClient.setQueryData<{ prefs: SeriesPlaybackPrefs | null }>(['series-prefs', seriesId], (old) => {
              if (!old) return old
              if (!old.prefs) {
                return {
                  ...old,
                  prefs: {
                    ...(patch as SeriesPlaybackPrefs),
                    series_id: seriesId,
                    updated_at: new Date().toISOString(),
                  },
                }
              }
              return { ...old, prefs: { ...old.prefs, ...(patch as SeriesPlaybackPrefs) } }
            })
          }
          queryClient.invalidateQueries({ queryKey: ['prefs', video.id] })
          if (seriesId) queryClient.invalidateQueries({ queryKey: ['series-prefs', seriesId] })
        })
        .catch(() => {})
    },
    [video.id, queryClient, prefs.data?.series_id],
  )

  // flushVolume persists the player's current volume/mute. Only a real user
  // adjustment is ever saved (never an auto-applied remembered value); it is
  // called as the unmount safety net and clears any pending debounce (Bug #4).
  const flushVolume = useCallback(() => {
    if (!userChangedVolumeRef.current) return
    if (volumeTimerRef.current !== undefined) {
      window.clearTimeout(volumeTimerRef.current)
      volumeTimerRef.current = undefined
    }
    const el = playerRef.current
    if (el && typeof el.volume === 'number') {
      savePrefs({ volume: el.volume, muted: el.muted })
    }
  }, [savePrefs])

  // saveHistory writes a resume position, swallowing the network error (history
  // is best-effort). Used by the throttled progress save, the ended marker and
  // the unmount safety net.
  const saveHistory = useCallback((progress: number) => putHistory(video.id, progress).catch(() => {}), [video.id])

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
      const save = posRef.current > 0 ? saveHistory(posRef.current) : Promise.resolve()
      void save.then(() => {
        queryClient.invalidateQueries({ queryKey: ['history', video.id] })
        if (video.show_id) queryClient.invalidateQueries({ queryKey: ['series'] })
      })
      // Flush a volume change made right before leaving (the 400ms debounce may
      // not have fired yet). Only a real user adjustment is saved, never an
      // auto-applied remembered value.
      flushVolume()
    },
    [video.id, video.show_id, queryClient, savePrefs, flushVolume, saveHistory],
  )

  // Auto-pause when the user switches away from the player tab: the player's
  // component tree stays alive (keep-alive), so the media would otherwise keep
  // playing in the background like an uncontrolled browser tab.
  useEffect(
    () =>
      subscribeTabs(() => {
        if (getActiveTab() !== 'player') {
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
  const mediaSrc = useMemo<StreamSrc>(() => {
    if (mode === 'transcode') {
      return { src: hlsPlaylistUrl(video.id, hlsSession, audio), type: 'application/x-mpegurl' }
    }
    return { src: mode === 'remux' ? remuxUrl(video.id, audio) : streamUrl(video.id), type: 'video/mp4' }
  }, [mode, video.id, hlsSession, audio])

  // persistProgress throttles history writes (every SAVE_INTERVAL) while playing.
  // Writes stop in the final NEAR_END seconds so the ending is not treated as a
  // resume point.
  function persistProgress(currentTime: number) {
    posRef.current = currentTime
    const dur = video.duration || 0
    if (dur > 0 && currentTime >= dur - NEAR_END) {
      return
    }
    if (currentTime - lastSaveRef.current >= SAVE_INTERVAL) {
      lastSaveRef.current = currentTime
      void saveHistory(currentTime)
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
      // Volume/muted are applied by passing them as controlled props to
      // <MediaPlayer> (Vidstack resets the media volume to 100% on can-play
      // unless a volume prop is given, so remote.changeVolume alone loses the
      // fight). Adopt the remembered values once; onVolumeChange then keeps the
      // props in sync with user adjustments (Bug #2).
      if (!volumeAdoptedRef.current && (typeof p.volume === 'number' || typeof p.muted === 'boolean')) {
        volumeAdoptedRef.current = true
        if (typeof p.volume === 'number') setPlayerVolume(p.volume)
        if (typeof p.muted === 'boolean') setPlayerMuted(p.muted)
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
  // is suppressed by applyingPrefsRef. A debounce waits for the volume to settle
  // (a slider drag changes it on every pointer move), then persists the final
  // value — the value at press is not meaningful, and this does not depend on
  // brittle pointer/class detection (Bug #4 修订).
  function onVolumeChange(detail: { volume: number; muted: boolean }) {
    if (applyingPrefsRef.current) return
    userChangedVolumeRef.current = true
    // Keep the controlled props in sync with the user's adjustment so the player
    // never snaps back to the remembered value on the next prop application.
    setPlayerVolume(detail.volume)
    setPlayerMuted(detail.muted)
    if (volumeTimerRef.current !== undefined) window.clearTimeout(volumeTimerRef.current)
    volumeTimerRef.current = window.setTimeout(() => {
      volumeTimerRef.current = undefined
      savePrefs({ volume: detail.volume, muted: detail.muted })
    }, 400)
  }

  return (
    <div ref={webFullscreen.attach} className="relative h-full w-full">
      <MediaPlayer
        ref={playerRef}
        src={mediaSrc}
        volume={playerVolume}
        muted={playerMuted}
        poster={coverUrl(video.id)}
        title={video.title}
        playsInline
        className="h-full w-full bg-black"
        style={{ aspectRatio: 'auto' }}
        onProviderChange={onProviderChange}
        onFullscreenChange={() => {
          // If the browser managed to enter native fullscreen while the web
          // overlay is already up (e.g. the native "f" shortcut or double-tap),
          // back out of native and restore the web overlay so they never stack.
          if (webFullscreen.active) {
            remote.exitFullscreen()
            webFullscreen.enter()
          }
        }}
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
          persistProgress(detail.currentTime)
        }}
        onPlaying={() => {
          wasPlayingRef.current = true
        }}
        onPause={() => {
          wasPlayingRef.current = false
        }}
        onEnded={() => {
          // Mark the video as fully watched by storing the full duration (Bug #3).
          // progress == duration distinguishes "completed" from "never played"
          // (progress 0, no row) everywhere, and resume logic skips it because it
          // falls outside the RESUME window, so replay starts from the beginning.
          posRef.current = video.duration || 0
          void saveHistory(posRef.current)
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
            // The project does not use Google Cast; drop the layout's cast
            // button so it never appears on supported (Chrome) clients.
            googleCastButton: null,
            beforeFullscreenButton: (
              <PlayerWebFullscreenButton active={webFullscreen.active} onToggle={toggleWebFullscreen} />
            ),
            // Native fullscreen is desktop-only: on mobile the web fullscreen
            // button already covers the CSS overlay path, and native fullscreen
            // would hide our custom controls and subtitles. Rendering null here
            // drops the native button from the layout on mobile.
            fullscreenButton: webFullscreen.mobile ? null : <PlayerNativeFullscreenButton onToggle={toggleNativeFullscreen} />,
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
        <FullscreenClock webActive={webFullscreen.active} />
      </MediaPlayer>
    </div>
  )
}
