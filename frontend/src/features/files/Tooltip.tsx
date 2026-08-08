import { useCallback, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'

// Tooltip shows an instant, app-styled bubble on hover/focus, replacing the
// delayed native title tooltip. It renders through a portal and clamps to the
// viewport, flipping below the trigger when there is no room above. The bubble
// is measured first (invisible) so it never jumps on first hover.
export function Tooltip({ tip, children }: { tip: string; children: ReactNode }) {
  const [trigger, setTrigger] = useState<DOMRect | null>(null)
  const [size, setSize] = useState<{ w: number; h: number } | null>(null)
  const wrapRef = useRef<HTMLSpanElement>(null)

  const measure = useCallback((node: HTMLDivElement | null) => {
    if (node) setSize({ w: node.offsetWidth, h: node.offsetHeight })
  }, [])

  function show() {
    const r = wrapRef.current?.getBoundingClientRect()
    if (r) setTrigger(r)
  }
  function hide() {
    setTrigger(null)
    setSize(null)
  }

  const gap = 8
  let pos: { left: number; top: number } | undefined
  if (trigger && size) {
    const left = Math.min(Math.max(trigger.left + trigger.width / 2 - size.w / 2, gap), window.innerWidth - size.w - gap)
    let top = trigger.top - size.h - gap
    if (top < gap) top = trigger.bottom + gap
    pos = { left, top }
  }

  return (
    <span ref={wrapRef} onMouseEnter={show} onMouseLeave={hide} onFocus={show} onBlur={hide} className="inline-flex">
      {children}
      {trigger &&
        createPortal(
          <div
            ref={measure}
            role="tooltip"
            style={pos}
            className={`pointer-events-none fixed z-[100] max-w-[70vw] rounded-lg border border-neutral-200 bg-white px-2.5 py-1 text-[13px] leading-5 text-neutral-800 shadow-xl ${
              size ? 'opacity-100' : 'opacity-0'
            }`}
          >
            {tip}
          </div>,
          document.body,
        )}
    </span>
  )
}
