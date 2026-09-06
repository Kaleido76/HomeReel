import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

// Web fullscreen fills the browser viewport with the player via pure CSS
// instead of the native Fullscreen API, so the browser chrome stays visible and
// the app's own controls / WebVTT subtitles are never hidden behind the native
// overlay (ADR-006). The player wrapper is stretched over the dynamic viewport;
// the overlay must be fixed (covering the tab bar too), so none of the player's
// ancestors may carry a CSS transform (a transformed ancestor would become the
// fixed element's containing block).
export function useWebFullscreen() {
  const [active, setActive] = useState(false)
  // The player DOM node to stretch. It is attached by the caller once the
  // <MediaPlayer> mounts (its root element), so enter/exit always act on a live
  // node.
  const nodeRef = useRef<HTMLElement | null>(null)

  const attach = useCallback((el: HTMLElement | null) => {
    nodeRef.current = el
  }, [])

  // Whether the device is touch-first (coarse pointer). It only decides which
  // fullscreen controls the layout shows: mobile keeps the web fullscreen
  // button and drops the native one, desktop shows both. Web fullscreen itself
  // behaves identically on every device.
  const mobile = useIsMobile()

  const enter = useCallback(() => {
    const el = nodeRef.current
    if (!el) return
    el.classList.add('web-fullscreen')
    setActive(true)
  }, [])

  const exit = useCallback(() => {
    const el = nodeRef.current
    if (!el) return
    el.classList.remove('web-fullscreen')
    setActive(false)
  }, [])

  const toggle = useCallback(() => {
    if (active) exit()
    else enter()
  }, [active, enter, exit])

  // Escape ends web fullscreen just like native fullscreen, so users are never
  // trapped. (On a phone the browser's back gesture may also close it.)
  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') exit()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [active, exit])

  return useMemo(
    () => ({
      active,
      mobile,
      attach,
      enter,
      exit,
      toggle,
    }),
    [active, mobile, attach, enter, exit, toggle],
  )
}

const coarsePointerQuery = '(pointer: coarse)'

function useIsMobile() {
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia(coarsePointerQuery).matches,
  )

  useEffect(() => {
    const mql = window.matchMedia(coarsePointerQuery)
    const onChange = (e: MediaQueryListEvent) => setIsMobile(e.matches)
    setIsMobile(mql.matches)
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  return isMobile
}
