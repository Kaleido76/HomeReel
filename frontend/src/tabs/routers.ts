import { lazy } from 'react'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { TAB_ROOTS, tabFromPath, type TabId } from './config'

type AnyRouter = ReturnType<typeof createRouter>

// Lazy page components: each tab's page chunks are loaded on first visit only
// ("懒加载"), then kept alive by the tab host.
const HomePage = lazy(() => import('../features/home/HomePage').then((m) => ({ default: m.HomePage })))
const LibraryLayout = lazy(() => import('../features/library/LibraryLayout').then((m) => ({ default: m.LibraryLayout })))
const PlayerPage = lazy(() => import('../features/player/PlayerPage').then((m) => ({ default: m.PlayerPage })))
const ToolsPage = lazy(() => import('../features/tools/ToolsPage').then((m) => ({ default: m.ToolsPage })))
const FilesPage = lazy(() =>
  import('../features/files/FilesPage').then((m) => ({ default: m.FilesPage })),
)

// initialEntry seeds each tab's memory history. The tab matching the current URL
// starts at that URL (refresh / deep-link support); every other tab starts at its
// own root and is only mounted when first activated.
function initialEntry(tab: TabId): string {
  if (tabFromPath(window.location.pathname) === tab) {
    return window.location.pathname + window.location.search
  }
  return TAB_ROOTS[tab]
}

// ---- home tab ----
const homeRoot = createRootRoute()
const homeIndex = createRoute({ getParentRoute: () => homeRoot, path: '/', component: HomePage })
const homeNotFound = createRoute({
  getParentRoute: () => homeRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/' }),
  component: () => null,
})
const homeTree = homeRoot.addChildren([homeIndex, homeNotFound])
export const homeRouter = createRouter({
  routeTree: homeTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('home')] }),
  defaultPreload: 'intent',
})

// ---- library tab (library browse / video detail / series detail) ----
// The root route carries LibraryLayout, which is the single source of truth
// for the wide-screen two-column browser (browse | detail). The child routes
// below only exist so the URL stays parseable and deep links / back-forward
// resolve; their components are never rendered via <Outlet/> — LibraryLayout
// parses location.pathname itself and renders the matching panes directly (the
// natural route IDs are preserved so tabFromPath keeps working).
const libraryRoot = createRootRoute({ component: LibraryLayout })
const libraryIndex = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/library',
  component: () => null,
})
const libraryVideo = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/library/video/$id',
  component: () => null,
})
const librarySeries = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/series/$id',
  component: () => null,
})
// /video is a child of the series route so that
// /series/:id/video/:videoId (episode detail inside a series) keeps the
// parent match — LibraryLayout parses the pathname into its column stack itself.
const librarySeriesVideo = createRoute({
  getParentRoute: () => librarySeries,
  path: 'video/$videoId',
  component: () => null,
})
const libraryNotFound = createRoute({
  getParentRoute: () => libraryRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/library' }),
  component: () => null,
})
const libraryTree = libraryRoot.addChildren([
  libraryIndex,
  libraryVideo,
  librarySeries.addChildren([librarySeriesVideo]),
  libraryNotFound,
])
export const libraryRouter = createRouter({
  routeTree: libraryTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('library')] }),
  defaultPreload: 'intent',
})

// ---- player tab (standalone playback page) ----
// The player owns its own tab so playback never competes with the library
// columns. /player is the tab root (empty state); /player/:videoId plays one
// video, with an optional ?series= carrying the series context that drives
// prev/next/up-next (a series member entered without the context still plays,
// it just has no neighbours).
const playerRoot = createRootRoute()
const playerIndex = createRoute({
  getParentRoute: () => playerRoot,
  path: '/player',
  component: PlayerPage,
  validateSearch: () => ({}),
})
const playerVideo = createRoute({
  getParentRoute: () => playerRoot,
  path: '/player/$videoId',
  component: PlayerPage,
  validateSearch: (search) => ({ series: typeof search.series === 'string' ? search.series : undefined }),
})
const playerNotFound = createRoute({
  getParentRoute: () => playerRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/player' }),
  component: () => null,
})
const playerTree = playerRoot.addChildren([playerIndex, playerVideo, playerNotFound])
export const playerRouter = createRouter({
  routeTree: playerTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('player')] }),
  defaultPreload: 'intent',
})

// ---- tools tab (工具: 左侧工具栏 + 右侧工具面板，如格式工厂) ----
const toolsRoot = createRootRoute()
const toolsIndex = createRoute({
  getParentRoute: () => toolsRoot,
  path: '/tools',
  component: ToolsPage,
  validateSearch: (search) => ({ tool: typeof search.tool === 'string' ? search.tool : '' }),
})
const toolsNotFound = createRoute({
  getParentRoute: () => toolsRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/tools' }),
  component: () => null,
})
const toolsTree = toolsRoot.addChildren([toolsIndex, toolsNotFound])
export const toolsRouter = createRouter({
  routeTree: toolsTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('tools')] }),
  defaultPreload: 'intent',
})

// ---- files tab (generic machine-wide file browser) ----
// The current absolute directory is kept in the URL so refresh/deep-link
// restores the exact folder being browsed.
const filesRoot = createRootRoute()
const filesIndex = createRoute({
  getParentRoute: () => filesRoot,
  path: '/files',
  component: FilesPage,
  validateSearch: (search) => ({ path: typeof search.path === 'string' ? search.path : '' }),
})
const filesNotFound = createRoute({
  getParentRoute: () => filesRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/files' }),
  component: () => null,
})
const filesTree = filesRoot.addChildren([filesIndex, filesNotFound])
export const filesRouter = createRouter({
  routeTree: filesTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('files')] }),
  defaultPreload: 'intent',
})

// Routers are typed with distinct route trees; Router's generics are invariant
// (the `update` method), so unify to a common AnyRouter at this single boundary.
export const routers: Record<TabId, AnyRouter> = {
  home: homeRouter as unknown as AnyRouter,
  library: libraryRouter as unknown as AnyRouter,
  player: playerRouter as unknown as AnyRouter,
  tools: toolsRouter as unknown as AnyRouter,
  files: filesRouter as unknown as AnyRouter,
}
