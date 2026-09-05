import { useRef } from 'react'

// useWindowDrag runs a pointer drag gesture. begin(e) starts tracking; while
// the pointer is held, handler(dx, dy, ev) is called with the movement relative
// to the drag start; onEnd runs once the pointer is released. Listeners live on
// window so the drag survives leaving the handle. Uses Pointer Events so
// touch, mouse and pen input all work identically.
export function useWindowDrag(
  handler: (dx: number, dy: number, ev: PointerEvent) => void,
  onEnd?: () => void,
): (e: React.PointerEvent) => void {
  const start = useRef<{ x: number; y: number; pointerId: number } | null>(null)
  const handlerRef = useRef(handler)
  const onEndRef = useRef(onEnd)
  handlerRef.current = handler
  onEndRef.current = onEnd

  return (e: React.PointerEvent) => {
    e.preventDefault()
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    start.current = { x: e.clientX, y: e.clientY, pointerId: e.pointerId }

    const onMove = (ev: PointerEvent) => {
      if (!start.current || ev.pointerId !== start.current.pointerId) return
      handlerRef.current(ev.clientX - start.current.x, ev.clientY - start.current.y, ev)
    }
    const onUp = (ev: PointerEvent) => {
      if (!start.current || ev.pointerId !== start.current.pointerId) return
      start.current = null
      target.releasePointerCapture(ev.pointerId)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      onEndRef.current?.()
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }
}
