import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

// Fake fullscreen (ADR-006 修订, 2026-09): on touch/mobile browsers native
// fullscreen is either unavailable (iOS Safari <div>) or takes over the whole
// browser UI, hiding our custom controls and WebVTT subtitles. Instead we fake
// it with CSS: the player wrapper is stretched to the dynamic viewport and
// (landscape content on a portrait device) force-rotated 90 degrees so the
// video fills the screen. A fixed overlay covers the app (including the tab
// bar), so the browser chrome is the only thing left visible.
//
// isCoarsePointer matches touch devices (the mobile target). A touch-capable
// laptop stays on native fullscreen because it has a fine pointer too.
const coarsePointerQuery = '(pointer: coarse)'

export function useFakeFullscreen(landscape: boolean) {
  const [active, setActive] = useState(false)
  // The player DOM node to stretch/rotate. It is attached by the caller once the
  // <MediaPlayer> mounts (its root element), so enter/exit always act on a live
  // node.
  const nodeRef = useRef<HTMLElement | null>(null)
  // Whether the video content itself is landscape (width > height). Rotation is
  // decided from the content's aspect, not the (portrait) player container —
  // measuring the container would wrongly skip rotation on a portrait phone.
  const landscapeRef = useRef(landscape)
  landscapeRef.current = landscape

  const attach = useCallback((el: HTMLElement | null) => {
    nodeRef.current = el
  }, [])

  const mobile = useIsMobile()

  // Enter fake fullscreen. A CSS transform on an ancestor would break the
  // position:fixed overlay (the ancestor becomes the containing block), but the
  // player's ancestors here carry no transform, so a fixed child reliably
  // covers the viewport. Rotation is the forced (CSS) landscape mode: on a
  // portrait device with landscape content we rotate 90 degrees so the video
  // fills the screen width-wise (ADR-006 修订) — it does not depend on the
  // physical orientation beyond the portrait check.
  const enter = useCallback(() => {
    const el = nodeRef.current
    if (!el) return
    el.classList.add('fake-fullscreen')
    if (window.matchMedia('(orientation: portrait)').matches && landscapeRef.current) {
      el.classList.add('fake-fullscreen-rotate')
    }
    setActive(true)
  }, [])

  const exit = useCallback(() => {
    const el = nodeRef.current
    if (!el) return
    el.classList.remove('fake-fullscreen', 'fake-fullscreen-rotate')
    setActive(false)
  }, [])

  const toggle = useCallback(() => {
    if (active) exit()
    else enter()
  }, [active, enter, exit])

  // Escape ends fake fullscreen just like native fullscreen, so users are never
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
