import { routers } from './routers'
import { tabFromPath, type TabId } from './config'
import { setPending, type ConvertTarget } from '../features/tools/format/queue'
import { basename } from '../features/files/path'

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

// Cross-tab navigation: jump to a route owned by the library tab (video detail /
// series detail / library root). Used by cards shown inside other tabs (home,
// search).
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

// playVideo starts playback in the player tab. seriesId (when given) carries the
// series context that drives prev/next and the up-next list; a standalone video
// plays without it. Used by the detail pane and the series member rows.
export function playVideo(videoId: string, seriesId?: string) {
  setActive('player')
  if (seriesId) {
    void routers.player.navigate({
      to: '/player/$videoId',
      params: { videoId },
      search: { series: seriesId },
    })
  } else {
    void routers.player.navigate({ to: '/player/$videoId', params: { videoId } })
  }
}

// openFormat hands the file tab's current selection (files and/or folders) to
// the 格式工厂 tool inside the 工具 tab. The batch lives in a module store —
// transient by nature, a selection can be many long paths and a refresh only
// shows the job queue anyway.
export function openFormat(items: ConvertTarget[]) {
  setPending(items)
  setActive('tools')
  void routers.tools.navigate({ to: '/tools', search: { tool: 'format' } })
}

// openFormatVideo sends a single video file to the 格式工厂 (a file must be
// handled as an item, not a folder). Shared by the detail pane and the player.
export function openFormatVideo(path: string) {
  openFormat([{ path, name: basename(path), is_dir: false }])
}

// openFileLocation switches to the 文件 tab at a specific directory. Used by
// the detail page's file-path line to jump to where the source file lives.
export function openFileLocation(dirPath: string) {
  setActive('files')
  void routers.files.navigate({ to: '/files', search: { path: dirPath } })
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
