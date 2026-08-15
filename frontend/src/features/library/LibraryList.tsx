import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Film, Layers, Loader2 } from 'lucide-react'
import { fetchVideos } from '../../api/videos'
import { fetchSeries } from '../../api/series'
import { coverUrl } from '../../api/videos'
import { seriesPosterUrl } from '../../api/series'
import { formatBytes, formatDuration } from '../../lib/format'
import type { GridState, ListSelection } from './types'

const PAGE_SIZE = 100

// LibraryList is the browse column of the wide-screen three-column library: a
// dense horizontal list in YouTube-search style — every entry (standalone video
// or series) has the same fixed height with a 16:9 cover on the left and title +
// metadata on the right. Clicking a row selects it and fills the detail column.
export function LibraryList({
  state,
  onUpdate,
  selection,
  onSelect,
}: {
  state: GridState
  onUpdate: (next: Partial<GridState>) => void
  selection: ListSelection
  onSelect: (sel: NonNullable<ListSelection>) => void
}) {
  const q = state.q
  const showVideos = state.view !== 'series'
  const showSeries = state.view !== 'standalone'

  const videos = useQuery({
    queryKey: ['videos', 'library', q, state.tags, state.genre, state.year, state.sort, state.page],
    queryFn: () =>
      fetchVideos({
        q: q || undefined,
        genre: state.genre || undefined,
        year: state.year ? Number(state.year) : undefined,
        tags: state.tags,
        ungrouped: true,
        sort: state.sort,
        order: 'desc',
        page: state.page,
        pageSize: PAGE_SIZE,
      }),
    placeholderData: (prev) => prev,
    enabled: showVideos,
  })

  const series = useQuery({
    queryKey: ['series', q, state.tags, state.genre, state.year],
    queryFn: () =>
      fetchSeries({
        genre: state.genre || undefined,
        year: state.year ? Number(state.year) : undefined,
        tags: state.tags,
      }),
    enabled: showSeries,
  })

  const seriesItems = useMemo(() => {
    if (!series.data) return []
    const query = q.trim().toLowerCase()
    const items = query
      ? series.data.series.filter((s) => s.name.toLowerCase().includes(query) || s.title.toLowerCase().includes(query))
      : series.data.series
    return items
  }, [series.data, q])

  const loading = showVideos ? videos.isLoading : series.isLoading
  const error = showVideos ? videos.error : series.error
  const hasItems =
    (showVideos ? (videos.data?.videos.length ?? 0) : 0) > 0 || (showSeries ? seriesItems.length : 0) > 0
  const total = videos.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const showPageControls = showVideos && pageCount > 1

  function emptyHint(): string {
    if (q || state.tags.length > 0 || state.genre || state.year) return '没有匹配的视频或系列'
    if (state.view === 'series') return '暂无系列。系列只能手动创建：在「文件」页签对媒体源内的文件夹点「标记为系列」，其直接一级视频文件即成为系列成员。'
    if (state.view === 'standalone') return '暂无单集视频。扫描会先以单集入库；归入系列的视频显示在「系列」中。'
    return '暂无视频。请先在「文件」页签中把存放媒体的目录标记为多媒体源并等待扫描完成。'
  }

  return (
    <div className="flex h-full flex-col">
      {loading ? (
        <div className="flex flex-1 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      ) : error ? (
        <div className="border-b border-red-100 bg-red-50 px-4 py-4 text-sm text-red-600">{error.message}</div>
      ) : !hasItems ? (
        <div className="flex flex-1 items-center justify-center px-6 text-center text-sm text-neutral-400">
          {emptyHint()}
        </div>
      ) : (
        <>
          <ul className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto px-3 py-3">
          {showSeries &&
            seriesItems.map((s) => {
              const active = selection?.type === 'series' && selection.id === s.id
              return (
                <li key={s.id}>
                  <button
                    onClick={() => onSelect({ type: 'series', id: s.id })}
                    className={`group relative flex w-full cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 text-left transition-all duration-200 hover:z-10 ${
                      active
                        ? 'border-blue-500 bg-blue-50 shadow-sm ring-1 ring-blue-500/30'
                        : 'border-neutral-200 bg-white shadow-sm hover:-translate-y-0.5 hover:border-blue-400/60 hover:bg-blue-50/40 hover:shadow-md'
                    }`}
                  >
                    <div className="relative h-20 w-36 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100">
                      {s.member_count > 0 ? (
                        <img src={seriesPosterUrl(s.id)} alt={s.name} loading="lazy" className="h-full w-full object-cover" />
                      ) : (
                        <div className="flex h-full w-full items-center justify-center text-neutral-300">
                          <Layers className="size-6" />
                        </div>
                      )}
                      {s.total_duration > 0 && (
                        <span className="absolute bottom-1 right-1 rounded bg-neutral-900/80 px-1 py-0.5 text-[10px] text-white">
                          {formatDuration(s.total_duration)}
                        </span>
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="line-clamp-1 text-sm font-medium text-neutral-800" title={s.name}>
                        {s.name}
                      </p>
                      <p className="mt-1 truncate text-xs text-neutral-400">
                        系列剧集 · {s.member_count} 个成员
                        {s.link_count > 0 ? ` · ${s.link_count} 个关联` : ''}
                      </p>
                    </div>
                  </button>
                </li>
              )
            })}
          {showVideos &&
            videos.data?.videos.map((v) => {
              const active = selection?.type === 'video' && selection.id === v.id
              const isEpisode = v.kind === 'episode' || v.episode_number != null
              const meta = isEpisode
                ? `S${String(v.season_number ?? 1).padStart(2, '0')}E${String(v.episode_number ?? 1).padStart(2, '0')}`
                : v.width
                  ? `${v.width}×${v.height}`
                  : ''
              return (
                <li key={v.id}>
                  <button
                    onClick={() => onSelect({ type: 'video', id: v.id })}
                    className={`group relative flex w-full cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 text-left transition-all duration-200 hover:z-10 ${
                      active
                        ? 'border-blue-500 bg-blue-50 shadow-sm ring-1 ring-blue-500/30'
                        : 'border-neutral-200 bg-white shadow-sm hover:-translate-y-0.5 hover:border-blue-400/60 hover:bg-blue-50/40 hover:shadow-md'
                    }`}
                  >
                    <div className="relative h-20 w-36 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100">
                      {v.thumb_path ? (
                        <img src={coverUrl(v.id, true)} alt={v.title} loading="lazy" className="h-full w-full object-cover" />
                      ) : (
                        <div className="flex h-full w-full items-center justify-center text-neutral-300">
                          <Film className="size-6" />
                        </div>
                      )}
                      {v.duration > 0 && (
                        <span className="absolute bottom-1 right-1 rounded bg-neutral-900/80 px-1 py-0.5 text-[10px] text-white">
                          {formatDuration(v.duration)}
                        </span>
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="line-clamp-1 text-sm font-medium text-neutral-800" title={v.title}>
                        {v.title}
                      </p>
                      <p className="mt-1 flex items-center gap-1.5 truncate text-xs text-neutral-400">
                        {isEpisode && (
                          <span className="shrink-0 rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] font-medium text-neutral-500">
                            剧集
                          </span>
                        )}
                        <span className="truncate">
                          {meta}
                          {meta && (v.size || v.duration) ? ' · ' : ''}
                          {formatBytes(v.size)}
                        </span>
                      </p>
                    </div>
                  </button>
                </li>
              )
            })}
          </ul>
          {showPageControls && (
            <div className="flex shrink-0 items-center justify-center gap-3 border-t border-neutral-100 bg-white px-4 py-2.5 text-sm">
              <button
                onClick={() => onUpdate({ page: Math.max(1, state.page - 1) })}
                disabled={state.page <= 1}
                className="rounded border border-neutral-200 bg-white px-3 py-1.5 text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
              >
                上一页
              </button>
              <span className="text-neutral-500">
                第 {state.page} / {pageCount} 页 · 共 {total} 个视频
              </span>
              <button
                onClick={() => onUpdate({ page: Math.min(pageCount, state.page + 1) })}
                disabled={state.page >= pageCount}
                className="rounded border border-neutral-200 bg-white px-3 py-1.5 text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
              >
                下一页
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
