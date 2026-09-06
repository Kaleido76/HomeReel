import { useEffect, useState, type ReactNode } from 'react'
import { ArrowLeft, Search } from 'lucide-react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { AdvancedFilter } from './AdvancedFilter'
import { LibraryList } from './LibraryList'
import { SeriesDetailPage } from '../series/SeriesDetailPage'
import { VideoDetailPane } from '../player/VideoDetailPane'
import { viewTabs, type GridState, type ListSelection } from './types'
import { useIsWide } from '../../lib/breakpoints'
import { NarrowBack } from '../../components/NarrowBack'

// parseGridSearch reads the grid state from the /library URL so a refresh or
// deep link restores the same filter/search the user had open.
function parseGridSearch(search: Record<string, unknown>): GridState {
  const view = search.view === 'series' ? 'series' : search.view === 'standalone' ? 'standalone' : 'all'
  const rawPage = typeof search.page === 'string' ? Number(search.page) : Number.NaN
  const tags = typeof search.tags === 'string' ? search.tags.split(',').filter(Boolean) : []
  return {
    view,
    q: typeof search.q === 'string' ? search.q : '',
    page: Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1,
    tags,
  }
}

// The wide-screen library is a column stack. Browse is always the bottom column;
// selecting a video/series pushes a detail column. The viewport shows only the
// top two columns (each 50vw) — earlier columns are pushed off-screen to the
// left via translateX. The stack itself is encoded in the URL path, so every
// state survives refresh / back-forward:
//
//   /library                            -> [browse]
//   /library/video/:id                  -> [browse][video-detail]
//   /series/:id                         -> [browse][series-detail]
//   /series/:id/video/:videoId          -> [browse][series-detail][video-detail]
//
// Each column carries its own path and its parent's path, so "back" is just a
// pop to the parent. The stack is extensible: future column types only need a
// new path segment + a renderer below. Playback no longer lives here — it moved
// to its own player tab (features/player/PlayerPage).
type Column =
  | { type: 'browse'; path: string }
  | { type: 'video-detail'; id: string; path: string; parent: string }
  | { type: 'series-detail'; id: string; path: string; parent: string }

function parseStack(pathname: string): Column[] {
  const browse: Column = { type: 'browse', path: '/library' }
  const video = pathname.match(/^\/library\/video\/([^/]+)$/)
  if (video) {
    const id = video[1]
    const detailPath = `/library/video/${id}`
    return [browse, { type: 'video-detail', id, path: detailPath, parent: browse.path }]
  }
  const seriesChild = pathname.match(/^\/series\/([^/]+)\/video\/([^/]+)$/)
  if (seriesChild) {
    const seriesId = seriesChild[1]
    const videoId = seriesChild[2]
    const seriesPath = `/series/${seriesId}`
    const seriesCol: Column = { type: 'series-detail', id: seriesId, path: seriesPath, parent: browse.path }
    return [browse, seriesCol, { type: 'video-detail', id: videoId, path: pathname, parent: seriesPath }]
  }
  const series = pathname.match(/^\/series\/([^/]+)$/)
  if (series) {
    const id = series[1]
    return [browse, { type: 'series-detail', id, path: pathname, parent: browse.path }]
  }
  return [browse]
}

// LibraryLayout is the root component of the library tab. It parses the URL
// path into a column stack and renders it. The grid's filter state (view / q /
// sort / page) lives here in component state (keep-alive safe) and is written
// back to the /library URL only while the browse column is the top column.
export function LibraryLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const pathname = location.pathname
  const wide = useIsWide()
  const stack = parseStack(pathname)
  const onGridRoute = stack.length === 1

  const [grid, setGrid] = useState<GridState>(() => parseGridSearch(location.search ?? {}))
  const [viewMenuOpen, setViewMenuOpen] = useState(false)

  useEffect(() => {
    if (onGridRoute) {
      setGrid(parseGridSearch(location.search ?? {}))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.search, onGridRoute])

  // The browse list highlights the first detail column on the stack (the item
  // the user drilled into), which survives even when browse is pushed off-screen.
  const selCol = stack[1]
  let selection: ListSelection = null
  if (selCol?.type === 'video-detail') selection = { type: 'video', id: selCol.id }
  else if (selCol?.type === 'series-detail') selection = { type: 'series', id: selCol.id }

  function update(next: Partial<GridState>) {
    const merged = { ...grid, ...next }
    setGrid(merged)
    if (onGridRoute) {
      const { tags, ...rest } = merged
      void navigate({ to: '/library', search: { ...rest, tags: tags.join(',') } })
    }
  }

  // goPath is the stack-aware "pop to parent": returning to the browse root
  // carries the current grid state so the filter/search/page survive the trip.
  function goPath(href: string) {
    if (href === '/library') {
      const { tags, ...rest } = grid
      void navigate({ to: '/library', search: { ...rest, tags: tags.join(',') } })
    } else void navigate({ href })
  }

  function onSelect(sel: NonNullable<ListSelection>) {
    if (sel.type === 'video') void navigate({ to: '/library/video/$id', params: { id: sel.id } })
    else void navigate({ to: '/series/$id', params: { id: sel.id } })
  }

  const filterBar = (
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-200 bg-white px-4 py-2">
      {/* Wide screens: three tabs side by side. */}
      <div className="hidden w-fit gap-0.5 rounded border border-neutral-200 bg-white p-0.5 lg:flex">
        {viewTabs.map((t) => (
          <button
            key={t.value}
            onClick={() => update({ view: t.value })}
            className={
              grid.view === t.value
                ? 'rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white'
                : 'rounded px-3 py-1 text-sm text-neutral-600 hover:bg-neutral-100'
            }
          >
            {t.label}
          </button>
        ))}
      </div>
      {/* Narrow screens: the three choices collapse into one dropdown so the
          search box and filter button keep their room. */}
      <div className="relative lg:hidden">
        <button
          onClick={() => setViewMenuOpen((o) => !o)}
          aria-expanded={viewMenuOpen}
          className="flex items-center rounded border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-700"
        >
          {viewTabs.find((t) => t.value === grid.view)?.label}
        </button>
        {viewMenuOpen && (
          <div className="absolute left-0 top-full z-50 mt-1 w-32 overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-lg">
            {viewTabs.map((t) => (
              <button
                key={t.value}
                onClick={() => {
                  update({ view: t.value })
                  setViewMenuOpen(false)
                }}
                className={`block w-full px-4 py-3 text-left text-base ${
                  grid.view === t.value
                    ? 'bg-blue-50 font-medium text-blue-700'
                    : 'text-neutral-600 hover:bg-neutral-50'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        )}
      </div>
      {/* Search box: instant filtering — every keystroke filters directly,
          no submit/confirm step. */}
      <div className="flex min-w-0 flex-1 items-center gap-2 rounded border border-neutral-200 bg-white px-2.5 py-1.5">
        <Search className="size-4 shrink-0 text-neutral-400" />
        <input
          value={grid.q}
          onChange={(e) => update({ q: e.target.value })}
          placeholder="搜索系列或单集标题…"
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
        />
      </div>
      <AdvancedFilter filters={grid} onApply={(f) => update({ ...f, page: 1 })} />
    </div>
  )

  const top = stack[stack.length - 1]

  // ---- wide: render the whole stack, slide it so the top two columns are on screen ----
  // translateX = -(depth - 2) * 50vw, so [browse] alone shows browse + hint,
  // depth 2 shows [browse][detail], depth 3 shows [series][video-detail].
  const translate = -Math.max(0, stack.length - 2) * 50

  return (
    <div className="relative flex h-full flex-col">
      {wide ? (
        <div className="relative min-h-0 flex-1 overflow-hidden">
          <div
            className="flex h-full transition-transform duration-300 ease-out"
            style={{ transform: `translateX(${translate}vw)` }}
          >
            {/* browse column (index 0): the filter bar is part of it, so it slides
                off-screen along with the column when the user opens a panel. */}
            <div className="flex h-full w-[50vw] shrink-0 flex-col">
              {filterBar}
              <div className="min-h-0 flex-1">
                <LibraryList state={grid} onUpdate={update} selection={selection} onSelect={onSelect} />
              </div>
            </div>

            {stack.slice(1).map((col) => (
              <div
                key={colKey(col)}
                className="h-full w-[50vw] shrink-0 overflow-y-auto border-l border-neutral-200 bg-neutral-50"
              >
                {columnContent(col, goPath)}
              </div>
            ))}

            {stack.length === 1 && (
              <div className="flex h-full w-[50vw] shrink-0 items-center justify-center border-l border-neutral-200 bg-neutral-50 p-6 text-sm text-neutral-400">
                点击左侧条目查看详情
              </div>
            )}
          </div>
        </div>
      ) : (
        // ---- narrow: single full-page column with back navigation ----
        <div className="flex h-full flex-col">
          {onGridRoute && filterBar}
          <div className="min-h-0 flex-1 overflow-y-auto">
            {top.type === 'video-detail' ? (
              <>
                <NarrowBack label={backLabel(top.parent)} onBack={() => goPath(top.parent)} />
                <VideoDetailPane videoId={top.id} seriesScoped={top.parent.startsWith('/series/')} />
              </>
            ) : top.type === 'series-detail' ? (
              <>
                <NarrowBack label={backLabel(top.parent)} onBack={() => goPath(top.parent)} />
                <SeriesDetailPage seriesId={top.id} />
              </>
            ) : (
              <div className="h-full py-4">
                <LibraryList state={grid} onUpdate={update} selection={selection} onSelect={onSelect} />
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function colKey(col: Column): string {
  switch (col.type) {
    case 'browse':
      return 'browse'
    case 'video-detail':
      return `video-${col.id}`
    case 'series-detail':
      return `series-${col.id}`
  }
}

// columnContent renders one detail column. The 「返回系列」 bar is drawn only
// for a video-detail opened inside a series, so the way back is always the
// series context's top bar.
function columnContent(col: Column, goPath: (href: string) => void): ReactNode {
  if (col.type === 'video-detail' && col.parent.startsWith('/series/')) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex shrink-0 items-center gap-2 border-b border-neutral-200 bg-white px-3 py-2">
          <button
            onClick={() => goPath(col.parent)}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
          >
            <ArrowLeft className="size-4" /> 返回系列
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          <VideoDetailPane videoId={col.id} seriesScoped />
        </div>
      </div>
    )
  }
  switch (col.type) {
    case 'video-detail':
      return <VideoDetailPane videoId={col.id} />
    case 'series-detail':
      return <SeriesDetailPage seriesId={col.id} />
    case 'browse':
      return null
  }
}

function backLabel(parent: string): string {
  if (parent === '/library') return '返回视频库'
  if (parent.startsWith('/series/')) return '返回系列'
  return '返回详情'
}
