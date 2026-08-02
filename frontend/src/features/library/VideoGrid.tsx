import { useState } from 'react'
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { Loader2, Search } from 'lucide-react'
import type { Storage } from '../../api/storages'
import { fetchStorages } from '../../api/storages'
import { fetchVideos } from '../../api/videos'
import { VideoCard } from './VideoCard'

const PAGE_SIZE = 48

const sortOptions = [
  { value: 'date', label: '最近添加' },
  { value: 'title', label: '标题' },
  { value: 'duration', label: '时长' },
  { value: 'name', label: '文件名' },
  { value: 'rating', label: '评分' },
] as const

type SortValue = (typeof sortOptions)[number]['value']

// StandaloneGrid lists videos that are not part of any series (the 单集 view).
// Its filter/sort/page state lives in the library URL (?q=&sort=&page=) so it
// survives refresh, in-tab back/forward and view switching.
export function StandaloneGrid({ emptyHint }: { emptyHint?: string }) {
  const location = useLocation()
  const navigate = useNavigate()
  const search = (location.search ?? {}) as { view?: string; q?: string; sort?: string; page?: number }
  const [input, setInput] = useState(search.q ?? '')
  const query = search.q ?? ''
  const sort = (sortOptions as readonly { value: string }[]).some((o) => o.value === search.sort)
    ? (search.sort as SortValue)
    : 'date'
  const page = typeof search.page === 'number' && search.page > 0 ? search.page : 1

  const videos = useQuery({
    queryKey: ['videos', 'standalone', query, sort, page],
    queryFn: () =>
      fetchVideos({
        q: query || undefined,
        ungrouped: true,
        sort,
        order: 'desc',
        page,
        pageSize: PAGE_SIZE,
      }),
    placeholderData: (prev) => prev,
  })

  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })
  const storageById = useMemo(() => {
    const map = new Map<string, Storage>()
    for (const s of storages.data?.storages ?? []) map.set(s.id, s)
    return map
  }, [storages.data])

  const total = videos.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  function update(next: Partial<{ q: string; sort: SortValue; page: number }>) {
    navigate({
      to: '/library',
      search: {
        view: search.view ?? 'standalone',
        q: next.q ?? query,
        sort: next.sort ?? sort,
        page: next.page ?? page,
      },
    })
  }

  function submitSearch() {
    update({ q: input.trim(), page: 1 })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            submitSearch()
          }}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-neutral-200 bg-white px-3 py-2"
        >
          <Search className="size-4 shrink-0 text-neutral-400" />
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="搜索标题或文件名…"
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
          />
        </form>
        <select
          value={sort}
          onChange={(e) => {
            update({ sort: e.target.value as SortValue, page: 1 })
          }}
          className="rounded-lg border border-neutral-200 bg-white px-2.5 py-2 text-sm text-neutral-700 outline-none focus:border-indigo-400"
        >
          {sortOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      {videos.isLoading && (
        <div className="flex h-64 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      )}

      {videos.isError && (
        <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
          {videos.error.message}
        </div>
      )}

      {videos.data && (
        <>
          {videos.data.videos.length === 0 ? (
            <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
              {emptyHint ?? (query ? '没有匹配的视频' : '暂无单集视频。归入系列的视频会显示在「系列」中。')}
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
              {videos.data.videos.map((v) => (
                <VideoCard key={v.id} video={v} storage={storageById.get(v.storage_id)} />
              ))}
            </div>
          )}

          {pageCount > 1 && (
            <div className="flex items-center justify-center gap-3 text-sm">
              <button
                onClick={() => update({ page: Math.max(1, page - 1) })}
                disabled={page <= 1}
                className="rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
              >
                上一页
              </button>
              <span className="text-neutral-500">
                第 {page} / {pageCount} 页 · 共 {total} 个视频
              </span>
              <button
                onClick={() => update({ page: Math.min(pageCount, page + 1) })}
                disabled={page >= pageCount}
                className="rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
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
