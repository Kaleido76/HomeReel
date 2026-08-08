import { useRef } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'

// useWindowDrag runs a pointer drag gesture. begin(e) starts tracking; while
// the pointer is held, handler(dx, dy, ev) is called with the movement relative
// to the drag start; onEnd runs once the pointer is released. Listeners live on
// window so the drag survives leaving the handle.
export function useWindowDrag(
  handler: (dx: number, dy: number, ev: MouseEvent) => void,
  onEnd?: () => void,
): (e: ReactMouseEvent) => void {
  const start = useRef<{ x: number; y: number } | null>(null)
  const handlerRef = useRef(handler)
  const onEndRef = useRef(onEnd)
  handlerRef.current = handler
  onEndRef.current = onEnd

  return (e: ReactMouseEvent) => {
    e.preventDefault()
    start.current = { x: e.clientX, y: e.clientY }
    const onMove = (ev: MouseEvent) => {
      if (!start.current) return
      handlerRef.current(ev.clientX - start.current.x, ev.clientY - start.current.y, ev)
    }
    const onUp = () => {
      start.current = null
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      onEndRef.current?.()
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
}
