import { routers } from './routers'
import { TAB_ROOTS, tabFromPath, type TabId } from './config'

// Module-level tab store shared by every tab router. `activeTab` changes are
// broadcast to React via useSyncExternalStore; routers stay mounted so switching
// tabs preserves each tab's component state (the "keep-alive" core).

let activeTab: TabId = tabFromPath(
  typeof window !== 'undefined' ? window.location.pathname : '/',
)
const listeners = new Set<() => void>()

export function getActiveTab(): TabId {
  return activeTab
}

export function subscribeTabs(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

function setActive(tab: TabId) {
  if (activeTab === tab) return
  activeTab = tab
  listeners.forEach((l) => l())
}

function currentHref(tab: TabId): string {
  return routers[tab].state.location.href
}

// activate switches the visible tab without pushing history (browser tabs don't
// record a history step for switching), then aligns the address bar with the
// target tab's current location.
export function activate(tab: TabId) {
  if (activeTab === tab) return
  setActive(tab)
  window.history.replaceState(null, '', currentHref(tab))
}

// Cross-tab navigation: jump to a route owned by the library tab (player /
// series / library root). Used by cards shown inside other tabs (home, search).
export function openVideo(id: string) {
  setActive('library')
  void routers.library.navigate({ to: '/library/video/$id', params: { id } })
}

export function openSeries(id: string) {
  setActive('library')
  void routers.library.navigate({ to: '/series/$id', params: { id } })
}

export function openLibrary() {
  setActive('library')
  void routers.library.navigate({ to: '/library', search: {} })
}

// backToHome returns the tab to its root view (the "回到页签主页" button).
export function backToHome(tab: TabId) {
  setActive(tab)
  void routers[tab].navigate({ to: TAB_ROOTS[tab], search: {} })
}

// initTabSync wires the browser history to the per-tab memory histories:
//  - the active tab's router mirrors its location into the address bar
//  - popstate (back/forward) resolves the URL to a tab and realigns its router
// Call once from a mounted component (TabSync).
export function initTabSync() {
  const onPop = () => {
    const tab = tabFromPath(window.location.pathname)
    setActive(tab)
    void routers[tab].navigate({
      href: window.location.pathname + window.location.search,
      replace: true,
    })
  }
  window.addEventListener('popstate', onPop)

  const unsubs = (Object.keys(routers) as TabId[]).map((id) =>
    routers[id].subscribe('onResolved', () => {
      if (id === activeTab) {
        const href = routers[id].state.location.href
        const current = window.location.pathname + window.location.search
        if (href !== current) {
          window.history.pushState(null, '', href)
        }
      }
    }),
  )

  return () => {
    window.removeEventListener('popstate', onPop)
    unsubs.forEach((u) => u())
  }
}
