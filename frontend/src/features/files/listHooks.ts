import { useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { useWindowDrag } from './drag'

const COL_WIDTH_KEY = 'files.colWidths'
const LEGACY_COL_WIDTH_KEY = 'filesnew.colWidths.v2'

interface ColWidths {
  name: number
  size: number
  mtime: number
}

type ColKey = keyof ColWidths

// Name is the primary content and gets the bulk of the width; size/mtime only
// need to sit loosely in the remaining space.
const DEFAULT_WIDTHS: ColWidths = { name: 480, size: 150, mtime: 220 }

function loadWidths(): ColWidths {
  try {
    let raw = localStorage.getItem(COL_WIDTH_KEY)
    if (!raw) {
      raw = localStorage.getItem(LEGACY_COL_WIDTH_KEY)
      if (raw) localStorage.removeItem(LEGACY_COL_WIDTH_KEY)
    }
    if (raw) return { ...DEFAULT_WIDTHS, ...JSON.parse(raw) as Partial<ColWidths> }
  } catch {
    // fall back to defaults
  }
  return DEFAULT_WIDTHS
}

// useColumnWidths persists the list's resizable column widths in localStorage
// (migrating the pre-rename key once) and drives the header drag-resize gesture.
export function useColumnWidths() {
  const [colWidths, setColWidths] = useState<ColWidths>(loadWidths)
  const dragCol = useRef<{ key: ColKey; startWidth: number } | null>(null)

  const beginDrag = useWindowDrag((dx) => {
    if (!dragCol.current) return
    const { key, startWidth } = dragCol.current
    const min = key === 'name' ? 120 : 80
    setColWidths((prev) => {
      const next = { ...prev, [key]: Math.max(min, startWidth + dx) }
      try {
        localStorage.setItem(COL_WIDTH_KEY, JSON.stringify(next))
      } catch {
        // ignore quota errors
      }
      return next
    })
  })

  function onHeaderDrag(key: ColKey, e: ReactMouseEvent) {
    dragCol.current = { key, startWidth: colWidths[key] }
    beginDrag(e)
  }

  return { colWidths, onHeaderDrag }
}

// useCheckboxDrag implements drag-selection in the checkbox column: pressing in
// the column and dragging up/down paints a contiguous range of rows to the
// starting row's state (Windows Explorer style). Rows are resolved from the
// pointer position via the data-path attribute, so the range follows the
// visible (sorted) order. Movement must exceed a small threshold, otherwise the
// plain click toggle still applies.
export function useCheckboxDrag(
  paths: string[],
  selected: Set<string>,
  onSelectSet: (next: Set<string>) => void,
) {
  const orderRef = useRef<Map<string, number>>(new Map())
  const dragSelect = useRef<{ startPath: string; targetChecked: boolean } | null>(null)
  const baseCheck = useRef<Map<string, boolean> | null>(null)
  const suppressClick = useRef(false)
  orderRef.current = new Map(paths.map((p, i) => [p, i]))

  const beginCheckDrag = useWindowDrag(
    (dx, dy, ev) => {
      const d = dragSelect.current
      if (!d || Math.abs(dx) + Math.abs(dy) <= 4) return
      suppressClick.current = true
      const base = baseCheck.current
      if (!base) return
      const el = document.elementFromPoint(ev.clientX, ev.clientY) as HTMLElement | null
      const row = el?.closest('[data-path]') as HTMLElement | null
      const cur = row ? orderRef.current.get(row.dataset.path ?? '') : undefined
      const start = orderRef.current.get(d.startPath)
      if (cur === undefined || start === undefined) return
      const [lo, hi] = start <= cur ? [start, cur] : [cur, start]
      const next = new Set<string>()
      for (const [p, on] of base) if (on) next.add(p)
      for (const p of base.keys()) {
        const i = orderRef.current.get(p)
        if (i !== undefined && i >= lo && i <= hi) {
          if (d.targetChecked) next.add(p)
          else next.delete(p)
        }
      }
      onSelectSet(next)
    },
    () => {
      dragSelect.current = null
      baseCheck.current = null
    },
  )

  // A drag ends with a click at the release point; suppress that one click so
  // it cannot also toggle a row / navigate / single-select.
  function suppressIfDragged(): boolean {
    if (suppressClick.current) {
      suppressClick.current = false
      return true
    }
    return false
  }

  function onCheckboxMouseDown(path: string, ev: ReactMouseEvent) {
    ev.stopPropagation()
    suppressClick.current = false
    dragSelect.current = { startPath: path, targetChecked: !selected.has(path) }
    baseCheck.current = new Map(paths.map((p) => [p, selected.has(p)]))
    beginCheckDrag(ev)
  }

  return { suppressIfDragged, onCheckboxMouseDown }
}
