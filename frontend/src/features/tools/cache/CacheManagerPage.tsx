import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { fetchCacheStats } from '../../../api/cache'
import { fetchSeries } from '../../../api/series'
import { fetchVideos, type Video } from '../../../api/videos'
import { useJobs } from '../../jobs/useJobs'
import { useIsWide } from '../../../lib/breakpoints'
import { buildCacheModel, hasCache, hasStandaloneCache, type CacheSelection, type SeriesCacheInfo } from './model'
import { CacheOverviewBar } from './CacheOverviewBar'
import { CacheList } from './CacheList'
import { SeriesCacheDetail, StandaloneCacheDetail } from './CacheDetail'

// fetchAllStandaloneVideos returns every standalone (ungrouped) video, walking
// pages until the total is collected (home-scale libraries fit one or two).
async function fetchAllStandaloneVideos(): Promise<Video[]> {
  const out: Video[] = []
  const pageSize = 200
  for (let page = 1; page <= 20; page++) {
    const res = await fetchVideos({ ungrouped: true, pageSize, page })
    out.push(...res.videos)
    if (out.length >= res.total) break
  }
  return out
}

const sortByName = (a: SeriesCacheInfo, b: SeriesCacheInfo) => a.series.name.localeCompare(b.series.name, 'zh-Hans-CN')

// CacheManagerPage manages the regenerable caches around videos. It is a
// hierarchical, series-first view instead of the old flat dump: a compact
// overview strip, a searchable list of the series that own cache (with a
// one-line「显示无缓存的系列」toggle at the bottom), every standalone video
// (library 单集 semantics) and the orphan/remux global caches as pinned list
// items, plus a detail pane per series/video holding the fine-grained
// operations. Series-level batch actions (预生成 / 清理字幕 / 清理记忆) live in
// the detail; pre-generation runs as a standard background job. Clearing never
// touches source files.
export function CacheManagerPage() {
  const queryClient = useQueryClient()
  const [selection, setSelection] = useState<CacheSelection | null>(null)
  const [query, setQuery] = useState('')
  const [showAll, setShowAll] = useState(false)
  const [showStandaloneAll, setShowStandaloneAll] = useState(false)
  const wide = useIsWide()

  const seriesQuery = useQuery({ queryKey: ['series'], queryFn: () => fetchSeries() })
  const cacheQuery = useQuery({ queryKey: ['cache'], queryFn: fetchCacheStats })
  // Every standalone video (not just the cached ones) so the「未归组单集」section
  // works like the library's 单集 view — a no-cache video can still be managed
  // (e.g. pre-generate its subtitles).
  const standaloneVideosQuery = useQuery({ queryKey: ['videos', 'ungrouped'], queryFn: fetchAllStandaloneVideos })

  const model = useMemo(
    () =>
      buildCacheModel(cacheQuery.data, seriesQuery.data?.series ?? [], standaloneVideosQuery.data ?? []),
    [cacheQuery.data, seriesQuery.data, standaloneVideosQuery.data],
  )

  // A finished pregen job changed the cache underneath the page: refetch once so
  // the shown state matches what is on disk.
  const jobsQuery = useJobs()
  const invalidatedPregen = useRef(new Set<string>())
  useEffect(() => {
    for (const j of jobsQuery.data?.jobs ?? []) {
      if (j.type !== 'pregen') continue
      if (j.status === 'done' || j.status === 'failed') {
        if (!invalidatedPregen.current.has(j.id)) {
          invalidatedPregen.current.add(j.id)
          void queryClient.invalidateQueries({ queryKey: ['cache'] })
        }
      }
    }
  }, [jobsQuery.data, queryClient])

  const q = query.trim().toLowerCase()
  const cachedSeries = useMemo(() => model.series.filter(hasCache), [model.series])
  const noCacheSeries = useMemo(() => model.series.filter((s) => !hasCache(s)), [model.series])
  const shownSeries = useMemo(
    () => (showAll ? [...cachedSeries, ...noCacheSeries] : cachedSeries).sort(sortByName),
    [cachedSeries, noCacheSeries, showAll],
  )
  const filteredSeries = useMemo(
    () => (q ? shownSeries.filter((s) => s.series.name.toLowerCase().includes(q)) : shownSeries),
    [shownSeries, q],
  )
  const cachedStandalone = useMemo(() => model.standalone.filter(hasStandaloneCache), [model.standalone])
  const noCacheStandalone = useMemo(() => model.standalone.filter((s) => !hasStandaloneCache(s)), [model.standalone])
  const filteredStandalone = useMemo(() => {
    const shown = showStandaloneAll
      ? [...cachedStandalone, ...noCacheStandalone]
      : cachedStandalone
    return q ? shown.filter((s) => s.title.toLowerCase().includes(q)) : shown
  }, [cachedStandalone, noCacheStandalone, showStandaloneAll, q])

  // On wide screens keep a selection among the visible items (first row by
  // default); on narrow screens nothing is auto-selected so the list stays open.
  const visibleItems = useMemo<CacheSelection[]>(() => {
    const out: CacheSelection[] = filteredSeries.map((s) => ({ kind: 'series' as const, id: s.series.id }))
    for (const st of filteredStandalone) out.push({ kind: 'standalone' as const, id: st.videoId })
    return out
  }, [filteredSeries, filteredStandalone])

  useEffect(() => {
    if (!wide) return
    const visible = new Set(visibleItems.map((s) => `${s.kind}:${s.id}`))
    if (selection && visible.has(`${selection.kind}:${selection.id}`)) return
    setSelection(visibleItems[0] ?? null)
  }, [wide, selection, visibleItems])

  async function onChanged() {
    await queryClient.invalidateQueries({ queryKey: ['cache'] })
  }

  if (seriesQuery.isLoading || cacheQuery.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (seriesQuery.isError || cacheQuery.isError) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600">
          {cacheQuery.error instanceof Error ? cacheQuery.error.message : '加载缓存数据失败'}
        </p>
      </div>
    )
  }

  const showList = !selection || wide
  const showDetail = !!selection

  return (
    <div className="flex min-h-full flex-col gap-4 lg:h-full lg:min-h-0">
      <CacheOverviewBar overview={cacheQuery.data} />

      <div className="flex min-h-0 flex-1 gap-4">
        <div className={`${showList ? 'flex min-h-0 min-w-0 flex-col' : 'hidden'} ${wide ? 'w-96 shrink-0' : 'flex-1'}`}>
          <CacheList
            series={filteredSeries}
            standalone={filteredStandalone}
            noCacheCount={noCacheSeries.length}
            showAll={showAll}
            onToggleShowAll={() => setShowAll((v) => !v)}
            standaloneNoCacheCount={noCacheStandalone.length}
            standaloneShowAll={showStandaloneAll}
            onToggleStandaloneShowAll={() => setShowStandaloneAll((v) => !v)}
            query={query}
            onQuery={setQuery}
            selection={selection}
            onSelect={setSelection}
            orphans={cacheQuery.data?.orphans}
            onChanged={onChanged}
          />
        </div>

        {showDetail && (
          <div className="min-h-0 flex-1 overflow-y-auto">
            {selection.kind === 'series' ? (
              <SeriesCacheDetail
                info={model.series.find((s) => s.series.id === selection.id)!}
                subtitlesByVideo={model.subtitlesByVideo}
                remuxByVideo={model.remuxByVideo}
                prefsByVideo={model.prefsByVideo}
                seriesPrefs={model.seriesPrefs.get(selection.id)}
                narrow={!wide}
                onBack={() => setSelection(null)}
                onChanged={onChanged}
              />
            ) : (
              <StandaloneCacheDetail
                item={model.standalone.find((s) => s.videoId === selection.id)!}
                narrow={!wide}
                onBack={() => setSelection(null)}
                onChanged={onChanged}
              />
            )}
          </div>
        )}

        {!showDetail && wide && (
          <div className="hidden min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-neutral-200 bg-white text-sm text-neutral-400 lg:flex">
            选择左侧的系列或单集查看缓存详情
          </div>
        )}
      </div>
    </div>
  )
}
