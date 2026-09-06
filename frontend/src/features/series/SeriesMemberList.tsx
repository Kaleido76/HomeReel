import { useCallback, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDownAZ, ArrowUpNarrowWide, GripVertical, Info, Pencil, Play, X } from 'lucide-react'
import { reorderSeriesMembers, resortSeries, type SeriesMember } from '../../api/series'
import { formatDuration } from '../../lib/format'
import { playMode } from '../../lib/playability'
import { RESUME_TAIL } from '../../lib/playback'
import { playVideo } from '../../tabs/manager'
import { ProgressBar } from '../../components/ProgressBar'
import { Tooltip } from '../../components/Tooltip'
import { SeriesRenameModal } from './SeriesRenameModal'

// Drag threshold before a press becomes a drag (a plain click must not reorder).
const DRAG_THRESHOLD = 6
// The middle band of a row is a swap target; the top/bottom quarter edges are
// insert gaps (drag onto another episode → swap, into a gap → insert).
const SWAP_BAND = 0.25

type Target = { type: 'swap'; index: number } | { type: 'insert'; index: number; y: number }

// SeriesMemberList renders the series' episode list with drag-to-reorder
// (ADR-015 修订). The header row carries the title (left) and an expandable
// button group (right): a 6-dot grip that toggles edit mode (drag enabled +
// `select-none`), plus — once expanded — 「按当前名称排序」「恢复文件名序」and
// 「批量修改显示名称」. Dragging lifts a ghost that looks like the row (no episode
// number) and follows the pointer, while the original row stays in place — so
// hover/target detection is unaffected. The target position highlights (swap
// band or insert line) and release persists the order via POST /api/series/{id}/order.
// The number slot is not a drag handle and stays a visually separate column.
export function SeriesMemberList({ seriesId, members }: { seriesId: string; members: SeriesMember[] }) {
  const queryClient = useQueryClient()
  const listRef = useRef<HTMLDivElement>(null)
  const rowRefs = useRef<(HTMLDivElement | null)[]>([])
  const pending = useRef<{ index: number; pointerId: number; startX: number; startY: number; active: boolean } | null>(null)
  // drag/target live in refs so onPointerUp never reads a stale closure; the
  // render version forces a repaint on every move. startRect anchors the ghost
  // to the dragged row's original position, offsetY moves it with the pointer.
  // The ghost is absolutely positioned inside the list container (its nearest
  // positioned ancestor), because the wide-screen column stack applies a
  // transform that would shift a position:fixed element's coordinate origin.
  const dragRef = useRef<{ index: number; clientY: number } | null>(null)
  const startRectRef = useRef<DOMRect | null>(null)
  const listRectRef = useRef<DOMRect | null>(null)
  const targetRef = useRef<Target | null>(null)
  const [, setVersion] = useState(0)
  // sortMode gates drag-to-reorder: only while enabled rows can be dragged, and
  // the list is unselectable so the gesture never collapses into text selection.
  const [sortMode, setSortMode] = useState(false)
  const [renaming, setRenaming] = useState(false)

  // All three order mutations refresh the same detail/list caches after they land.
  const onOrderChanged = () => {
    void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
    void queryClient.invalidateQueries({ queryKey: ['series'] })
  }

  const reorder = useMutation({
    mutationFn: (ids: string[]) => reorderSeriesMembers(seriesId, ids),
    onSuccess: onOrderChanged,
  })

  // sortByName re-orders members by their current display name (显示名) via the
  // manual-order endpoint, so the list reflects the edited names instead of the
  // file-name order.
  const sortByName = useMutation({
    mutationFn: () => {
      const ids = [...members]
        .sort((a, b) => (a.episode_title || a.title).localeCompare(b.episode_title || b.title))
        .map((m) => m.video_id)
      return reorderSeriesMembers(seriesId, ids)
    },
    onSuccess: onOrderChanged,
  })

  const resort = useMutation({
    mutationFn: () => resortSeries(seriesId),
    onSuccess: () => {
      setSortMode(false)
      onOrderChanged()
    },
  })

  const computeTarget = useCallback(
    (clientY: number): Target | null => {
      if (!listRef.current) return null
      const rects = rowRefs.current.map((el) => el?.getBoundingClientRect()).filter((r): r is DOMRect => !!r)
      if (rects.length === 0) return null
      const listTop = listRef.current.getBoundingClientRect().top
      if (clientY < rects[0].top + rects[0].height * SWAP_BAND) {
        return { type: 'insert', index: 0, y: rects[0].top - listTop }
      }
      for (let i = 0; i < rects.length; i++) {
        const r = rects[i]
        const innerTop = r.top + r.height * SWAP_BAND
        const innerBottom = r.bottom - r.height * SWAP_BAND
        if (clientY < innerTop) {
          return { type: 'insert', index: i, y: r.top - listTop }
        }
        if (clientY <= innerBottom) {
          return { type: 'swap', index: i }
        }
      }
      const last = rects[rects.length - 1]
      return { type: 'insert', index: rects.length, y: last.bottom - listTop }
    },
    [],
  )

  const onPointerDown = (e: React.PointerEvent, index: number) => {
    if (!sortMode) return
    const el = e.target as HTMLElement
    if (el.closest('button, a, [data-no-drag]')) return
    pending.current = { index, pointerId: e.pointerId, startX: e.clientX, startY: e.clientY, active: false }
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  const onPointerMove = (e: React.PointerEvent) => {
    const p = pending.current
    if (!p || e.pointerId !== p.pointerId) return
    if (!p.active) {
      if (Math.hypot(e.clientX - p.startX, e.clientY - p.startY) < DRAG_THRESHOLD) return
      p.active = true
      startRectRef.current = rowRefs.current[p.index]?.getBoundingClientRect() ?? null
      listRectRef.current = listRef.current?.getBoundingClientRect() ?? null
    }
    dragRef.current = { index: p.index, clientY: e.clientY }
    targetRef.current = computeTarget(e.clientY)
    setVersion((v) => v + 1)
  }

  const onPointerUp = () => {
    const p = pending.current
    pending.current = null
    const drag = dragRef.current
    const target = targetRef.current
    dragRef.current = null
    startRectRef.current = null
    listRectRef.current = null
    targetRef.current = null
    setVersion((v) => v + 1)
    if (!p?.active || !drag || !target) return
    const ids = members.map((m) => m.video_id)
    const next = applyOrder(ids, drag.index, target)
    if (next) reorder.mutate(next)
  }

  const onPointerCancel = () => {
    pending.current = null
    dragRef.current = null
    startRectRef.current = null
    listRectRef.current = null
    targetRef.current = null
    setVersion((v) => v + 1)
  }

  const drag = dragRef.current
  const target = targetRef.current
  const startRect = startRectRef.current
  const listRect = listRectRef.current
  const dragMember = drag ? members[drag.index] : undefined

  return (
    <div>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium text-neutral-700">
          剧集列表
          <span className="ml-2 text-xs font-normal text-neutral-400">{members.length} 集</span>
        </h3>
        <div className="flex shrink-0 items-center gap-1 rounded-md border border-neutral-200 p-1">
          <Tooltip content="编辑模式">
            <button
              onClick={() => {
                pending.current = null
                dragRef.current = null
                startRectRef.current = null
                targetRef.current = null
                setSortMode((v) => !v)
              }}
              aria-pressed={sortMode}
              className={`flex items-center justify-center rounded px-1 py-0.5 transition-colors ${
                sortMode ? 'text-blue-600 hover:bg-blue-50' : 'text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700'
              }`}
            >
              <GripVertical className="size-4" />
            </button>
          </Tooltip>
          {sortMode && (
            <>
              <span className="mx-0.5 h-4 w-px bg-neutral-200" />
              <Tooltip content="按显示名排序">
                <button
                  onClick={() => sortByName.mutate()}
                  disabled={sortByName.isPending}
                  className="flex items-center justify-center rounded px-1 py-0.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 disabled:opacity-40"
                >
                  <ArrowUpNarrowWide className="size-4" />
                </button>
              </Tooltip>
              <Tooltip content="按文件名排序">
                <button
                  onClick={() => resort.mutate()}
                  disabled={resort.isPending}
                  className="flex items-center justify-center rounded px-1 py-0.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800 disabled:opacity-40"
                >
                  <ArrowDownAZ className="size-4" />
                </button>
              </Tooltip>
              <Tooltip content="批量修改显示名称">
                <button
                  onClick={() => setRenaming(true)}
                  className="flex items-center justify-center rounded px-1 py-0.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
                >
                  <Pencil className="size-4" />
                </button>
              </Tooltip>
              <Tooltip content="退出编辑模式">
                <button
                  onClick={() => setSortMode(false)}
                  className="flex items-center justify-center rounded px-1 py-0.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700"
                >
                  <X className="size-4" />
                </button>
              </Tooltip>
            </>
          )}
        </div>
      </div>
      <div ref={listRef} className={`relative space-y-1.5 ${sortMode ? 'select-none' : ''}`}>
        {members.map((member, i) => {
          const isDragging = drag?.index === i
          const isSwapTarget = target?.type === 'swap' && target.index === i
          return (
            <div
              key={member.video_id}
              ref={(el) => {
                rowRefs.current[i] = el
              }}
              onPointerDown={(e) => onPointerDown(e, i)}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              onPointerCancel={onPointerCancel}
            className={`flex items-stretch rounded-md border transition-colors ${
              isDragging
                ? 'border-neutral-200 bg-white opacity-30'
                : isSwapTarget
                  ? 'border-blue-500 bg-blue-100 ring-2 ring-blue-500/50'
                  : 'border-neutral-200 bg-white'
            }`}
            >
              <div
                data-no-drag
                className={`flex w-12 shrink-0 items-center justify-center self-stretch border-r text-sm font-medium transition-colors ${
                  isSwapTarget ? 'border-blue-500/50 bg-blue-100 text-blue-600' : 'border-neutral-100 bg-neutral-50 text-neutral-500'
                }`}
              >
                {member.episode_number}
              </div>
              <div className="min-w-0 flex-1 px-4 py-3.5">
                <p className="truncate text-sm font-medium text-neutral-800">{member.episode_title || member.title}</p>
                {member.duration > 0 && member.progress > 0 && (
                  <ProgressBar
                    value={Math.min(100, (member.progress / member.duration) * 100)}
                    className="mt-2 h-1 w-full max-w-xs"
                  />
                )}
              </div>
              <span className="flex shrink-0 items-center pr-4 text-xs text-neutral-400">
                {member.duration > 0 ? formatDuration(member.duration) : ''}
              </span>
              <div className="flex shrink-0 items-center gap-2 pr-4">
                {/* member carries both the probe metadata and the backend
                    capability flags playMode reads, so it serves as both args. */}
                {playMode(member, member) !== 'none' ? (
                  <button
                    onClick={() => playVideo(member.video_id, seriesId)}
                    className="flex shrink-0 items-center gap-1.5 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm text-neutral-700 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
                  >
                    <Play className="size-3.5" />{' '}
                    <span className="hidden sm:inline">
                      {member.progress > 0 && member.progress < member.duration - RESUME_TAIL ? '续播' : '播放'}
                    </span>
                  </button>
                ) : (
                  <Tooltip content="无法在线播放">
                    <button
                      disabled
                      className="flex shrink-0 cursor-not-allowed items-center gap-1.5 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-1.5 text-sm text-neutral-400"
                    >
                      <Play className="size-3.5" /> <span className="hidden sm:inline">不可播放</span>
                    </button>
                  </Tooltip>
                )}
                <Link
                  to="/series/$id/video/$videoId"
                  params={{ id: seriesId, videoId: member.video_id }}
                  className="flex shrink-0 items-center gap-1.5 rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-sm text-neutral-700 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
                >
                  <Info className="size-3.5" /> <span className="hidden sm:inline">详情</span>
                </Link>
              </div>
            </div>
          )
        })}
        {target?.type === 'insert' && (
          <div
            className="pointer-events-none absolute left-0 right-0 z-10 border-t-2 border-blue-500"
            style={{ top: target.y }}
          />
        )}
        {drag && startRect && listRect && dragMember && (
          <div
            className="pointer-events-none absolute z-50 flex items-center gap-3 rounded-md border border-neutral-200 bg-white px-4 py-3.5 opacity-60 shadow-xl"
          style={{
            left: startRect.left - listRect.left,
            top: drag.clientY - listRect.top - startRect.height / 2,
            width: startRect.width,
          }}
          >
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-neutral-800">{dragMember.episode_title || dragMember.title}</p>
                {dragMember.duration > 0 && dragMember.progress > 0 && (
                  <ProgressBar
                    value={Math.min(100, (dragMember.progress / dragMember.duration) * 100)}
                    className="mt-2 h-1 w-full max-w-xs"
                  />
                )}
              </div>
            <span className="shrink-0 text-xs text-neutral-400">
              {dragMember.duration > 0 ? formatDuration(dragMember.duration) : ''}
            </span>
          </div>
        )}
      </div>
      {renaming && <SeriesRenameModal seriesId={seriesId} members={members} onClose={() => setRenaming(false)} />}
    </div>
  )
}

// applyOrder returns the reordered video ids for a drop, or null when the drop
// leaves the order unchanged (dropping on itself / adjacent no-op).
function applyOrder(ids: string[], from: number, target: Target): string[] | null {
  const next = [...ids]
  if (target.type === 'swap') {
    if (target.index === from) return null
    const a = next[from]
    next[from] = next[target.index]
    next[target.index] = a
    return next
  }
  const [moved] = next.splice(from, 1)
  let at = target.index
  if (from < at) at -= 1
  if (at === from) return null
  next.splice(at, 0, moved)
  return next
}
