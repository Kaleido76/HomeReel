import { useEffect, useRef, useState } from 'react'
import {
  isHLSProvider,
  MediaPlayer,
  MediaProvider,
  Track,
  useMediaProvider,
  useMediaRemote,
  type HLSSrc,
  type MediaPlayerInstance,
  type VideoSrc,
} from '@vidstack/react'
import { DefaultVideoLayout, defaultLayoutIcons } from '@vidstack/react/player/layouts/default'
import '@vidstack/react/player/styles/base.css'
import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import type { Video } from '../../api/videos'
import { coverUrl, fetchHistory, hlsUrl, putHistory, streamUrl, subtitleUrl } from '../../api/videos'
import { getActiveTab, subscribeTabs } from '../../tabs/manager'

// Resume only kicks in beyond these boundaries so an almost-finished video
// starts fresh and a stray 1s glitch never seeks anywhere.
const RESUME_MIN = 10
const RESUME_TAIL = 20
// Progress persistence is throttled (ADR plan: ~10s cadence).
const SAVE_INTERVAL = 10

// HlsLibConfig pins the local hls.js package instead of Vidstack's default
// CDN load (LAN-only deployment) and relaxes the manifest timeout so the
// player survives the on-demand transcode's first-play latency instead of
// aborting at the 10s hls.js default and spinning forever.
//
// Buffering is capped tightly: during the incremental transcode the playlist
// has no ENDLIST, so hls.js treats it as a live stream and races toward the
// "live edge" — with a fast transcode the edge advances far ahead of playback
// and hls.js would otherwise download dozens of segments up front. A ~1-segment
// forward buffer (maxBufferLength) plus a short back buffer keep the download
// to a little-ahead preload; maxBufferSize 0 disables the byte cap so the time
// cap is the single binding limit.
function HlsLibConfig() {
  const provider = useMediaProvider()
  useEffect(() => {
    if (provider && isHLSProvider(provider)) {
      provider.library = () => import('hls.js')
      provider.config = {
        manifestLoadingTimeOut: 60_000,
        manifestLoadingMaxRetry: 20,
        manifestLoadingRetryDelay: 2000,
        liveDurationInfinity: true,
        liveSyncDurationCount: 1,
        maxBufferLength: 10,
        maxMaxBufferLength: 10,
        maxBufferSize: 0,
        backBufferLength: 30,
      }
    }
  }, [provider])
  return null
}

export function VideoPlayer({
  video,
  directPlayable,
  hlsEnabled,
  onEnded,
}: {
  video: Video
  directPlayable: boolean
  hlsEnabled: boolean
  onEnded?: () => void
}) {
  const playerRef = useRef<MediaPlayerInstance>(null)
  const remote = useMediaRemote(playerRef)
  const [resumeAt, setResumeAt] = useState(0)
  const posRef = useRef(0)
  const lastSaveRef = useRef(0)

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
      if (posRef.current > 0) {
        void putHistory(video.id, posRef.current).catch(() => {})
      }
    },
    [video.id],
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

  const direct = directPlayable || !hlsEnabled
  // /api/stream/{id} has no extension, so Vidstack cannot infer the media type
  // and would fall back to a HEAD probe (which can fail on cancelled requests).
  // Provide the type explicitly to select the right loader.
  const mediaSrc: VideoSrc | HLSSrc = direct
    ? { src: streamUrl(video.id), type: 'video/mp4' }
    : { src: hlsUrl(video.id), type: 'application/x-mpegurl' }

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

  return (
    <MediaPlayer
      ref={playerRef}
      src={mediaSrc}
      poster={coverUrl(video.id)}
      title={video.title}
      playsInline
      className="h-full w-full bg-black"
      style={{ aspectRatio: 'auto' }}
      onCanPlay={() => {
        if (resumeAt > 0) {
          remote.seek(resumeAt)
          setResumeAt(0)
        }
      }}
      onTimeUpdate={onTimeUpdate}
      onEnded={() => {
        posRef.current = 0
        void putHistory(video.id, 0).catch(() => {})
        onEnded?.()
      }}
    >
      <HlsLibConfig />
      <MediaProvider>
        <Track src={subtitleUrl(video.id)} kind="subtitles" label="字幕" />
      </MediaProvider>
      <DefaultVideoLayout icons={defaultLayoutIcons} />
    </MediaPlayer>
  )
}
