import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Loader2, Play } from 'lucide-react'
import { fetchVideo } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import { VideoMetaPanel } from '../player/VideoMetaPanel'

// VideoDetailPane is the single-video detail shown in the middle column. It is
// deliberately compact: a play action, the editable metadata panel (title,
// tags, series assignment) and a technical summary line. The heavy player lives
// in the right-hand column (PlayerPane), which opens on play.
//
// playHref is resolved by the caller from the column stack: a detail opened
// inside a series plays within the series (/series/:id/play/:videoId), a
// standalone detail plays via /library/video/:id/play.
export function VideoDetailPane({ videoId, playHref }: { videoId: string; playHref: string }) {
  const navigate = useNavigate()
  const detail = useQuery({ queryKey: ['video', videoId], queryFn: () => fetchVideo(videoId) })

  if (detail.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {detail.error.message}
      </div>
    )
  }

  if (!detail.data) return null
  const video = detail.data.video

  return (
    <div className="space-y-4 p-4 sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium text-neutral-500">播放</span>
        <button
          onClick={() => navigate({ href: playHref })}
          className="flex items-center gap-1.5 rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
        >
          <Play className="size-4" /> 播放
        </button>
      </div>

      <div className="rounded-md border border-neutral-200 bg-white p-4">
        <VideoMetaPanel video={video} initialTags={detail.data.tags ?? []} />
      </div>

      <div className="rounded-md border border-neutral-200 bg-white px-4 py-3">
        <p className="text-sm text-neutral-500">
          {video.width && video.height ? `${video.width}×${video.height} · ` : ''}
          {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
          {formatBytes(video.size)} · {video.container?.toUpperCase()}
          {detail.data.series_id ? (
            <Link to="/series/$id" params={{ id: detail.data.series_id }} className="ml-2 text-blue-600 hover:underline">
              所属系列 →
            </Link>
          ) : null}
        </p>
      </div>
    </div>
  )
}
