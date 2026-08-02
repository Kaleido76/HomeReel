import { Link } from '@tanstack/react-router'
import { Layers, PlayCircle } from 'lucide-react'
import type { Series } from '../../api/series'
import { seriesPosterUrl } from '../../api/series'

export function SeriesCard({ series }: { series: Series }) {
  return (
    <Link
      to="/series/$id"
      params={{ id: series.id }}
      className="group relative flex flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white transition-shadow hover:shadow-md"
    >
      <div className="relative aspect-[2/3] w-full overflow-hidden bg-neutral-100">
        {series.member_count > 0 ? (
          <img
            src={seriesPosterUrl(series.id)}
            alt={series.name}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-neutral-300">
            <Layers className="size-10" />
          </div>
        )}
        <span className="absolute inset-0 flex items-center justify-center bg-black/0 opacity-0 transition-opacity group-hover:bg-black/20 group-hover:opacity-100">
          <PlayCircle className="size-12 text-white drop-shadow" />
        </span>
      </div>
      <div className="min-w-0 flex-1 px-3 py-2">
        <p className="truncate text-sm font-medium text-neutral-800" title={series.name}>
          {series.name}
        </p>
        <p className="mt-0.5 truncate text-xs text-neutral-400">
          {series.kind === 'movie' ? '电影部' : '剧集季'} · {series.member_count} 个成员
          {series.link_count > 0 ? ` · ${series.link_count} 个关联` : ''}
        </p>
      </div>
    </Link>
  )
}
