import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'

export type TooltipPlacement = 'top' | 'bottom' | 'left' | 'right'

const GAP = 8
const TOUCH_DISMISS_MS = 2000

function clamp(v: number, min: number, max: number) {
  return Math.min(Math.max(v, min), Math.max(min, max))
}

function position(
  anchor: DOMRect | null,
  size: { w: number; h: number } | null,
  placement: TooltipPlacement,
): { left: number; top: number } | undefined {
  if (!anchor || !size) return undefined
  const pad = GAP
  // clientWidth/clientHeight exclude the scrollbar that innerWidth/innerHeight
  // include: clamping against innerWidth would let the bubble sit ~scrollbar px
  // past the visible viewport, and shrink-to-fit would then wrap its text a few
  // px early — leaving exactly one CJK character alone on the second line.
  const vw = document.documentElement.clientWidth || window.innerWidth
  const vh = document.documentElement.clientHeight || window.innerHeight
  let left: number
  let top: number
  if (placement === 'top' || placement === 'bottom') {
    left = clamp(anchor.left + anchor.width / 2 - size.w / 2, pad, vw - size.w - pad)
    if (placement === 'top') {
      top = anchor.top - size.h - GAP
      if (top < pad) top = anchor.bottom + GAP
    } else {
      top = anchor.bottom + GAP
      if (top + size.h > vh - pad) top = anchor.top - size.h - GAP
    }
  } else {
    top = clamp(anchor.top + anchor.height / 2 - size.h / 2, pad, vh - size.h - pad)
    if (placement === 'left') {
      left = anchor.left - size.w - GAP
      if (left < pad) left = anchor.right + GAP
    } else {
      left = anchor.right + GAP
      if (left + size.w > vw - pad) left = anchor.left - size.w - GAP
    }
  }
  return { left, top }
}

// Tooltip is the system-wide hover hint: an instant (no native delay) dark
// bubble with a fade/translate in-out animation. It renders through a portal
// and clamps to the viewport, flipping to the opposite side when there is no
// room on the requested placement. The bubble is measured first (invisible)
// so it never jumps on first hover; an empty content renders nothing.
//
// The bubble is laid out with width: max-content (capped to the viewport via
// max-w-[calc(100vw - 2rem)]) so its width NEVER depends on the positioned
// `left`/available space — otherwise a bubble clamped near the right edge would
// be sized against the scrollbar-inflated innerWidth and its text would wrap a
// few px early, leaving a single orphan CJK character on the second line. The
// position clamp itself uses clientWidth/clientHeight (scrollbar excluded), so
// the positioned bubble always fits the visible viewport and matches the
// measured size exactly.
export function Tooltip({
  content,
  placement = 'top',
  children,
}: {
  content?: string
  placement?: TooltipPlacement
  children: ReactNode
}) {
  const [anchor, setAnchor] = useState<DOMRect | null>(null)
  const [size, setSize] = useState<{ w: number; h: number } | null>(null)
  const [shown, setShown] = useState(false)
  const wrapRef = useRef<HTMLSpanElement>(null)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const touchDismissTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isTouch = useRef(typeof window !== 'undefined' && window.matchMedia('(pointer: coarse)').matches)

  const measure = useCallback((node: HTMLDivElement | null) => {
    if (node) setSize({ w: node.offsetWidth, h: node.offsetHeight })
  }, [])

  useEffect(() => {
    if (!anchor || !size) return
    const raf = requestAnimationFrame(() => setShown(true))
    return () => cancelAnimationFrame(raf)
  }, [anchor, size])

  useEffect(
    () => () => {
      if (hideTimer.current) clearTimeout(hideTimer.current)
      if (touchDismissTimer.current) clearTimeout(touchDismissTimer.current)
    },
    [],
  )

  function show() {
    if (hideTimer.current) {
      clearTimeout(hideTimer.current)
      hideTimer.current = null
    }
    if (touchDismissTimer.current) {
      clearTimeout(touchDismissTimer.current)
      touchDismissTimer.current = null
    }
    const r = wrapRef.current?.getBoundingClientRect()
    if (r) {
      setAnchor(r)
      setShown(false)
      if (isTouch.current) {
        touchDismissTimer.current = setTimeout(() => hide(), TOUCH_DISMISS_MS)
      }
    }
  }
  function hide() {
    setShown(false)
    if (touchDismissTimer.current) {
      clearTimeout(touchDismissTimer.current)
      touchDismissTimer.current = null
    }
    hideTimer.current = setTimeout(() => setAnchor(null), 180)
  }

  if (!content) return <>{children}</>

  const pos = position(anchor, size, placement)
  const hiddenTransform =
    placement === 'top'
      ? 'translateY(4px)'
      : placement === 'bottom'
        ? 'translateY(-4px)'
        : placement === 'left'
          ? 'translateX(4px)'
          : 'translateX(-4px)'

  return (
    <span ref={wrapRef} onMouseEnter={show} onMouseLeave={hide} onFocus={show} onBlur={hide} className="inline-flex">
      {children}
      {anchor &&
        createPortal(
          <div
            ref={measure}
            role="tooltip"
            style={
              pos
                ? { left: pos.left, top: pos.top, width: 'max-content', transform: shown ? 'none' : hiddenTransform }
                : { width: 'max-content', transform: hiddenTransform }
            }
            className={`pointer-events-none fixed z-[100] max-w-[calc(100vw-2rem)] rounded-md border border-neutral-700 bg-neutral-900 px-2.5 py-1 text-[13px] leading-5 text-neutral-50 shadow-lg transition duration-150 ease-out ${
              shown ? 'opacity-100' : 'opacity-0'
            }`}
          >
            {content}
          </div>,
          document.body,
        )}
    </span>
  )
}
