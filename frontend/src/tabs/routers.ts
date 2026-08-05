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
const SearchPage = lazy(() => import('../features/search/SearchPage').then((m) => ({ default: m.SearchPage })))
const ExplorerPage = lazy(() => import('../features/explorer/ExplorerPage').then((m) => ({ default: m.ExplorerPage })))
const RemuxPage = lazy(() => import('../features/remux/RemuxPage').then((m) => ({ default: m.RemuxPage })))

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

// ---- library tab (library / video detail / player / series detail) ----
// The root route carries LibraryLayout, which is the single source of truth
// for the wide-screen three-column browser (browse | detail | player). The
// child routes below only exist so the URL stays parseable and deep links /
// back-forward resolve; their components are never rendered via <Outlet/> —
// LibraryLayout parses location.pathname itself and renders the matching panes
// directly (the natural route IDs are preserved so tabFromPath keeps working).
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
// /play is a child of the video route so that /library/video/:id/play keeps the
// parent match in the tree — useParams({ from: '/library/video/$id' }) resolves
// in both the detail and the playing state.
const libraryPlay = createRoute({
  getParentRoute: () => libraryVideo,
  path: 'play',
  component: () => null,
})
const librarySeries = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/series/$id',
  component: () => null,
})
// /video and /play are children of the series route so that
// /series/:id/video/:videoId (episode detail inside a series) and
// /series/:id/play/:videoId (playing an episode inside a series) keep the
// parent match — LibraryLayout parses the pathname into its column stack itself.
const librarySeriesVideo = createRoute({
  getParentRoute: () => librarySeries,
  path: 'video/$videoId',
  component: () => null,
})
const librarySeriesPlay = createRoute({
  getParentRoute: () => librarySeries,
  path: 'play/$videoId',
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
  libraryVideo.addChildren([libraryPlay]),
  librarySeries.addChildren([librarySeriesVideo, librarySeriesPlay]),
  libraryNotFound,
])
export const libraryRouter = createRouter({
  routeTree: libraryTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('library')] }),
  defaultPreload: 'intent',
})

// ---- search tab ----
const searchRoot = createRootRoute()
const searchIndex = createRoute({
  getParentRoute: () => searchRoot,
  path: '/search',
  component: SearchPage,
  validateSearch: (search) => ({ q: typeof search.q === 'string' ? search.q : '' }),
})
const searchNotFound = createRoute({
  getParentRoute: () => searchRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/search' }),
  component: () => null,
})
const searchTree = searchRoot.addChildren([searchIndex, searchNotFound])
export const searchRouter = createRouter({
  routeTree: searchTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('search')] }),
  defaultPreload: 'intent',
})

// ---- explorer tab ----
const explorerRoot = createRootRoute()
const explorerIndex = createRoute({
  getParentRoute: () => explorerRoot,
  path: '/explorer',
  component: ExplorerPage,
  validateSearch: (search) => ({
    storageId: typeof search.storageId === 'string' ? search.storageId : '',
    path: typeof search.path === 'string' ? search.path : '',
  }),
})
const explorerNotFound = createRoute({
  getParentRoute: () => explorerRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/explorer' }),
  component: () => null,
})
const explorerTree = explorerRoot.addChildren([explorerIndex, explorerNotFound])
export const explorerRouter = createRouter({
  routeTree: explorerTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('explorer')] }),
  defaultPreload: 'intent',
})

// ---- remux tab (segmented-MP4 remux management) ----
const remuxRoot = createRootRoute()
const remuxIndex = createRoute({
  getParentRoute: () => remuxRoot,
  path: '/remux',
  component: RemuxPage,
})
const remuxNotFound = createRoute({
  getParentRoute: () => remuxRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/remux' }),
  component: () => null,
})
const remuxTree = remuxRoot.addChildren([remuxIndex, remuxNotFound])
export const remuxRouter = createRouter({
  routeTree: remuxTree,
  history: createMemoryHistory({ initialEntries: [initialEntry('remux')] }),
  defaultPreload: 'intent',
})

// Routers are typed with distinct route trees; Router's generics are invariant
// (the `update` method), so unify to a common AnyRouter at this single boundary.
export const routers: Record<TabId, AnyRouter> = {
  home: homeRouter as unknown as AnyRouter,
  library: libraryRouter as unknown as AnyRouter,
  search: searchRouter as unknown as AnyRouter,
  explorer: explorerRouter as unknown as AnyRouter,
  remux: remuxRouter as unknown as AnyRouter,
}
