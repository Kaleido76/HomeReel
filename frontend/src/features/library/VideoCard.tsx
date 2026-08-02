import { Film, HardDrive, PlayCircle } from 'lucide-react'
import type { Storage } from '../../api/storages'
import type { Video } from '../../api/videos'
import { coverUrl } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import { openVideo } from '../../tabs/manager'

// VideoCard is the landscape (16:9, row-span-2) card of the dense wall. The
// image fills the available cell height (object-cover crops), the title wraps
// two lines so long series-style names stay readable, and the meta line adapts:
// episodes show their SxxEyy + episode title, standalone videos show
// resolution · size. Episodes (inside series) additionally get a kind chip.
export function VideoCard({ video, storage }: { video: Video; storage?: Storage }) {
  const offline = storage !== undefined && !storage.available
  const isEpisode = video.kind === 'episode' || video.episode_number != null
  const meta = isEpisode
    ? `S${String(video.season_number ?? 1).padStart(2, '0')}E${String(video.episode_number ?? 1).padStart(2, '0')}`
    : video.width
      ? `${video.width}×${video.height}`
      : ''

  return (
    <button
      onClick={() => openVideo(video.id)}
      className="group row-span-2 flex min-h-0 flex-col overflow-hidden rounded-lg border border-neutral-200 bg-white text-left transition-colors hover:border-neutral-300"
    >
      <div className="relative min-h-0 flex-1 overflow-hidden bg-neutral-100">
        {video.thumb_path ? (
          <img
            src={coverUrl(video.id, true)}
            alt={video.title}
            loading="lazy"
            className="absolute inset-0 h-full w-full object-cover"
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-neutral-300">
            <Film className="size-10" />
          </div>
        )}
        {offline && (
          <span className="absolute inset-0 flex items-center justify-center bg-neutral-100/80 text-neutral-500">
            <span className="flex items-center gap-1.5 rounded bg-white px-2.5 py-1 text-xs">
              <HardDrive className="size-3.5" /> 存储离线
            </span>
          </span>
        )}
        {video.duration > 0 && !offline && (
          <span className="absolute bottom-1.5 right-1.5 rounded bg-neutral-900/80 px-1.5 py-0.5 text-xs text-white">
            {formatDuration(video.duration)}
          </span>
        )}
        {!offline && (
          <span className="absolute inset-0 flex items-center justify-center bg-white/0 opacity-0 transition-opacity group-hover:bg-black/10 group-hover:opacity-100">
            <PlayCircle className="size-12 text-white" />
          </span>
        )}
      </div>
      <div className="min-w-0 px-3 pb-2.5 pt-2">
        <p className="line-clamp-2 text-sm font-medium leading-snug text-neutral-800" title={video.title}>
          {video.title}
        </p>
        <p className="mt-1 flex items-center gap-1.5 truncate text-xs text-neutral-400">
          {isEpisode && (
            <span className="shrink-0 rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] font-medium text-neutral-500">
              剧集
            </span>
          )}
          <span className="truncate">
            {meta}
            {meta && (video.size || video.duration) ? ' · ' : ''}
            {formatBytes(video.size)}
          </span>
        </p>
      </div>
    </button>
  )
}
