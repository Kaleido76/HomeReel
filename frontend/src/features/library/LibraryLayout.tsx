import { useEffect, useState, type ReactNode } from 'react'
import { ArrowLeft, Search } from 'lucide-react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { AdvancedFilter } from './AdvancedFilter'
import { LibraryList } from './LibraryList'
import { SeriesDetailPage } from '../series/SeriesDetailPage'
import { VideoDetailPane } from '../player/VideoDetailPane'
import { PlayerPane } from '../player/PlayerPane'
import { sortOptions, viewTabs, type GridState, type ListSelection } from './types'
import { useMediaQuery } from '../../lib/useMediaQuery'

const isSort = (v: string): v is GridState['sort'] =>
  (sortOptions as readonly unknown[] as readonly string[]).includes(v)

// parseGridSearch reads the grid state from the /library URL so a refresh or
// deep link restores the same filter/sort the user had open.
function parseGridSearch(search: Record<string, unknown>): GridState {
  const view = search.view === 'series' ? 'series' : search.view === 'standalone' ? 'standalone' : 'all'
  const rawPage = typeof search.page === 'string' ? Number(search.page) : Number.NaN
  const tags = typeof search.tags === 'string' ? search.tags.split(',').filter(Boolean) : []
  return {
    view,
    q: typeof search.q === 'string' ? search.q : '',
    sort: typeof search.sort === 'string' && isSort(search.sort) ? search.sort : 'date',
    page: Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1,
    tags,
    genre: typeof search.genre === 'string' ? search.genre : '',
    year: typeof search.year === 'string' ? search.year : '',
  }
}

// The wide-screen library is a column stack. Browse is always the bottom column;
// selecting a video/series pushes a detail column, and playing pushes a player
// column. The viewport shows only the top two columns (each 50vw) — earlier
// columns are pushed off-screen to the left via translateX. The stack itself is
// encoded in the URL path, so every state survives refresh / back-forward:
//
//   /library                            -> [browse]
//   /library/video/:id                  -> [browse][video-detail]
//   /library/video/:id/play             -> [browse][video-detail][player]
//   /series/:id                         -> [browse][series-detail]
//   /series/:id/video/:videoId          -> [browse][series-detail][video-detail]
//   /series/:id/play/:videoId           -> [browse][series-detail][player]
//
// Each column carries its own path and its parent's path, so "back" is just a
// pop to the parent. The stack is extensible: future column types only need a
// new path segment + a renderer below.
type Column =
  | { type: 'browse'; path: string }
  | { type: 'video-detail'; id: string; path: string; parent: string }
  | { type: 'series-detail'; id: string; path: string; parent: string }
  | { type: 'player'; id: string; path: string; parent: string }

function parseStack(pathname: string): Column[] {
  const browse: Column = { type: 'browse', path: '/library' }
  const video = pathname.match(/^\/library\/video\/([^/]+)(?:\/play)?$/)
  if (video) {
    const id = video[1]
    const playing = pathname.endsWith('/play')
    const detailPath = `/library/video/${id}`
    const detail: Column = { type: 'video-detail', id, path: detailPath, parent: browse.path }
    if (playing) return [browse, detail, { type: 'player', id, path: pathname, parent: detailPath }]
    return [browse, detail]
  }
  const seriesChild = pathname.match(/^\/series\/([^/]+)\/(video|play)\/([^/]+)$/)
  if (seriesChild) {
    const seriesId = seriesChild[1]
    const videoId = seriesChild[3]
    const seriesPath = `/series/${seriesId}`
    const seriesCol: Column = { type: 'series-detail', id: seriesId, path: seriesPath, parent: browse.path }
    const col: Column =
      seriesChild[2] === 'play'
        ? { type: 'player', id: videoId, path: pathname, parent: seriesPath }
        : { type: 'video-detail', id: videoId, path: pathname, parent: seriesPath }
    return [browse, seriesCol, col]
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
  const wide = useMediaQuery('(min-width: 1024px)')
  const stack = parseStack(pathname)
  const onGridRoute = stack.length === 1

  const [grid, setGrid] = useState<GridState>(() => parseGridSearch(location.search ?? {}))

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
      <div className="flex w-fit gap-0.5 rounded border border-neutral-200 bg-white p-0.5">
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
      <form
        onSubmit={(e) => {
          e.preventDefault()
          update({ q: grid.q.trim() })
        }}
        className="flex min-w-0 flex-1 items-center gap-2 rounded border border-neutral-200 bg-white px-2.5 py-1.5"
      >
        <Search className="size-4 shrink-0 text-neutral-400" />
        <input
          value={grid.q}
          onChange={(e) => setGrid({ ...grid, q: e.target.value })}
          onBlur={() => update({ q: grid.q.trim() })}
          placeholder="搜索标题或文件名…"
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
        />
      </form>
      <AdvancedFilter filters={grid} onApply={(f) => update({ ...f, page: 1 })} />
    </div>
  )

  const top = stack[stack.length - 1]

  // ---- narrow: single full-page column with back navigation ----
  if (!wide) {
    return (
      <div className="flex h-full flex-col">
        {onGridRoute && filterBar}
        <div className="min-h-0 flex-1 overflow-y-auto">
          {top.type === 'video-detail' ? (
            <>
              <NarrowBack label={backLabel(top.parent)} onBack={() => goPath(top.parent)} />
              <VideoDetailPane
                videoId={top.id}
                playHref={videoPlayHref(top.parent, top.id)}
                seriesScoped={top.parent.startsWith('/series/')}
              />
            </>
          ) : top.type === 'series-detail' ? (
            <>
              <NarrowBack label={backLabel(top.parent)} onBack={() => goPath(top.parent)} />
              <SeriesDetailPage seriesId={top.id} />
            </>
          ) : top.type === 'player' ? (
            <PlayerPane videoId={top.id} exitHref={top.parent} goHref={playerGo(top.parent)} />
          ) : (
            <div className="h-full py-4">
              <LibraryList state={grid} onUpdate={update} selection={selection} onSelect={onSelect} />
            </div>
          )}
        </div>
      </div>
    )
  }

  // ---- wide: render the whole stack, slide it so the top two columns are on screen ----
  // translateX = -(depth - 2) * 50vw, so [browse] alone shows browse + hint,
  // depth 2 shows [browse][detail], depth 3 shows [detail][player], etc.
  const translate = -Math.max(0, stack.length - 2) * 50

  return (
    <div className="flex h-full flex-col">
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
    case 'player':
      return `player-${col.id}`
  }
}

// columnContent renders one detail/player column. Navigation targets are
// derived from the column's parent in the stack (not from the video's own
// attributes), so the behaviour is fully determined by how the column was
// reached:
//
//   - video-detail over a series: play stays inside the series
//     (/series/:id/play/:videoId) and replaces this detail with the player;
//     a toolbar offers the way back to the series.
//   - video-detail over browse (standalone): play uses /library/video/:id/play.
//   - player over a series: exit pops back to the series detail, prev/next/
//     up-next URLs stay inside the series.
//   - player over a standalone video detail: exit pops back to that detail.
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
          <VideoDetailPane videoId={col.id} playHref={videoPlayHref(col.parent, col.id)} seriesScoped />
        </div>
      </div>
    )
  }
  switch (col.type) {
    case 'video-detail':
      return <VideoDetailPane videoId={col.id} playHref={videoPlayHref(col.parent, col.id)} />
    case 'series-detail':
      return <SeriesDetailPage seriesId={col.id} />
    case 'player':
      return <PlayerPane videoId={col.id} exitHref={col.parent} goHref={playerGo(col.parent)} />
    case 'browse':
      return null
  }
}

// videoPlayHref resolves the play target of a video-detail column from its
// parent in the stack: inside a series it stays within the series
// (/series/:id/play/:videoId); a standalone detail uses /library/video/:id/play.
function videoPlayHref(parent: string, id: string): string {
  return parent.startsWith('/series/') ? `${parent}/play/${id}` : `/library/video/${id}/play`
}

// playerGo builds the prev/next/up-next URLs for a player column. A player
// pushed on top of a series detail stays inside that series; a standalone
// player keeps the plain video route (never used, as standalone videos have
// no neighbours, but kept for safety).
function playerGo(parent: string): (id: string) => string {
  if (parent.startsWith('/series/')) return (id) => `${parent}/play/${id}`
  return (id) => `/library/video/${id}/play`
}

function backLabel(parent: string): string {
  if (parent === '/library') return '返回视频库'
  if (parent.startsWith('/series/')) return '返回系列'
  return '返回详情'
}

// NarrowBack is the single-column back link shown on narrow screens where the
// stack collapses to a single full-page column per level.
function NarrowBack({ label, onBack }: { label: string; onBack: () => void }) {
  return (
    <div className="flex items-center gap-2 border-b border-neutral-200 bg-white px-4 py-2">
      <button
        onClick={onBack}
        className="flex items-center gap-1.5 rounded px-2 py-1 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
      >
        <ArrowLeft className="size-4" /> {label}
      </button>
    </div>
  )
}
