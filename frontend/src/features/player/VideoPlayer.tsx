import { useEffect, useRef, useState } from 'react'
import {
  MediaPlayer,
  MediaProvider,
  Track,
  useMediaRemote,
  type MediaPlayerInstance,
  type VideoSrc,
} from '@vidstack/react'
import { useQuery } from '@tanstack/react-query'
import { DefaultVideoLayout, defaultLayoutIcons } from '@vidstack/react/player/layouts/default'
import '@vidstack/react/player/styles/base.css'
import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import type { Video } from '../../api/videos'
import { coverUrl, fetchHistory, fetchVideoSubtitles, putHistory, streamUrl, subtitleUrl } from '../../api/videos'
import { getActiveTab, subscribeTabs } from '../../tabs/manager'

// Resume only kicks in beyond these boundaries so an almost-finished video
// starts fresh and a stray 1s glitch never seeks anywhere.
const RESUME_MIN = 10
const RESUME_TAIL = 20
// Progress persistence is throttled (ADR plan: ~10s cadence).
const SAVE_INTERVAL = 10

// VideoPlayer plays over HTTP Range only (ADR-006, 2026-08): a file that the
// browser cannot decode natively is never handed to a Live/HLS path — the
// caller decides playability (canPlay in lib/playability.ts) and renders this
// player only for playable files, otherwise it shows the convert prompt.
export function VideoPlayer({
  video,
  onEnded,
}: {
  video: Video
  onEnded?: () => void
}) {
  const playerRef = useRef<MediaPlayerInstance>(null)
  const remote = useMediaRemote(playerRef)
  const [resumeAt, setResumeAt] = useState(0)
  const posRef = useRef(0)
  const lastSaveRef = useRef(0)

  // Subtitle track menu: the backend enumerates every playable subtitle source
  // (sidecar file + embedded text tracks) and each becomes a <Track>, so the
  // player's built-in menu offers switching between them.
  const subtitles = useQuery({ queryKey: ['subtitles', video.id], queryFn: () => fetchVideoSubtitles(video.id) })
  const subtitleTracks = subtitles.data?.subtitles

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

  // /api/stream/{id} has no extension, so Vidstack cannot infer the media type
  // and would fall back to a HEAD probe (which can fail on cancelled requests).
  // Provide the type explicitly to select the right loader.
  const mediaSrc: VideoSrc = { src: streamUrl(video.id), type: 'video/mp4' }

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
      <DefaultVideoLayout icons={defaultLayoutIcons} />
    </MediaPlayer>
  )
}
