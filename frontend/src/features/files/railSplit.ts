import { useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent, RefObject } from 'react'
import { useWindowDrag } from './drag'

const MIN_SPLIT = 0.15
const MAX_SPLIT = 0.85

function loadSplit(key: string, legacyKey: string, fallback: number): number {
  try {
    let raw = localStorage.getItem(key)
    if (!raw) {
      raw = localStorage.getItem(legacyKey)
      if (raw) localStorage.removeItem(legacyKey)
    }
    if (raw) {
      const n = Number(raw)
      if (Number.isFinite(n)) return Math.min(MAX_SPLIT, Math.max(MIN_SPLIT, n))
    }
  } catch {
    // fall through to default
  }
  return fallback
}

function saveSplit(key: string, v: number) {
  try {
    localStorage.setItem(key, String(v))
  } catch {
    // ignore quota errors
  }
}

// useRailSplit owns one draggable split boundary of the left rail: it loads the
// persisted ratio (migrating a pre-rename key once), drives the drag gesture
// against the container's height, and persists the new ratio. Each boundary is
// independent, so dragging one never nudges the others. containerRef points at
// the element whose height bounds the ratio.
export function useRailSplit<T extends HTMLElement>(
  storeKey: string,
  legacyKey: string,
  initial: number,
  containerRef: RefObject<T | null>,
) {
  const [ratio, setRatio] = useState(() => loadSplit(storeKey, legacyKey, initial))
  const dragStart = useRef<{ start: number } | null>(null)

  const beginDrag = useWindowDrag((_dx, dy) => {
    if (!dragStart.current || !containerRef.current) return
    const h = containerRef.current.getBoundingClientRect().height
    if (h <= 0) return
    const next = Math.min(MAX_SPLIT, Math.max(MIN_SPLIT, dragStart.current.start + dy / h))
    setRatio(next)
    saveSplit(storeKey, next)
  })

  function startDrag(e: ReactMouseEvent) {
    dragStart.current = { start: ratio }
    beginDrag(e)
  }

  return { ratio, startDrag }
}
