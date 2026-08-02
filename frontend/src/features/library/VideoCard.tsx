import { Link } from '@tanstack/react-router'
import { Film, HardDrive, PlayCircle } from 'lucide-react'
import type { Storage } from '../../api/storages'
import type { Video } from '../../api/videos'
import { coverUrl } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'

export function VideoCard({ video, storage }: { video: Video; storage?: Storage }) {
  const offline = storage !== undefined && !storage.available

  return (
    <Link
      to="/library/video/$id"
      params={{ id: video.id }}
      className="group flex flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white transition-shadow hover:shadow-md"
    >
      <div className="relative aspect-video w-full overflow-hidden bg-neutral-100">
        {video.thumb_path ? (
          <img
            src={coverUrl(video.id, true)}
            alt={video.title}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-neutral-300">
            <Film className="size-10" />
          </div>
        )}
        {offline && (
          <span className="absolute inset-0 flex items-center justify-center bg-neutral-900/50 text-white">
            <span className="flex items-center gap-1.5 rounded-full bg-black/50 px-2.5 py-1 text-xs">
              <HardDrive className="size-3.5" /> 存储离线
            </span>
          </span>
        )}
        {video.duration > 0 && !offline && (
          <span className="absolute bottom-1.5 right-1.5 rounded bg-black/60 px-1.5 py-0.5 text-xs text-white">
            {formatDuration(video.duration)}
          </span>
        )}
        {!offline && (
          <span className="absolute inset-0 flex items-center justify-center bg-black/0 opacity-0 transition-opacity group-hover:bg-black/20 group-hover:opacity-100">
            <PlayCircle className="size-12 text-white drop-shadow" />
          </span>
        )}
      </div>
      <div className="min-w-0 flex-1 px-3 py-2">
        <p className="truncate text-sm font-medium text-neutral-800" title={video.title}>
          {video.title}
        </p>
        <p className="mt-0.5 truncate text-xs text-neutral-400">
          {video.width ? `${video.width}×${video.height} · ` : ''}
          {formatBytes(video.size)}
        </p>
      </div>
    </Link>
  )
}
