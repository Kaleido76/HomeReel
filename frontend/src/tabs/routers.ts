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
const LibraryPage = lazy(() => import('../features/library/LibraryPage').then((m) => ({ default: m.LibraryPage })))
const PlayerPage = lazy(() => import('../features/player/PlayerPage').then((m) => ({ default: m.PlayerPage })))
const SeriesDetailPage = lazy(() =>
  import('../features/series/SeriesDetailPage').then((m) => ({ default: m.SeriesDetailPage })),
)
const SearchPage = lazy(() => import('../features/search/SearchPage').then((m) => ({ default: m.SearchPage })))
const ExplorerPage = lazy(() => import('../features/explorer/ExplorerPage').then((m) => ({ default: m.ExplorerPage })))

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

// ---- library tab (library / player / series) ----
const libraryRoot = createRootRoute()
const libraryIndex = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/library',
  component: LibraryPage,
  validateSearch: (search) => ({
    view: search.view === 'series' ? 'series' : 'standalone',
    q: typeof search.q === 'string' ? search.q : '',
    sort: typeof search.sort === 'string' ? search.sort : 'date',
    page: typeof search.page === 'string' && /^\d+$/.test(search.page) ? Number(search.page) : 1,
  }),
})
const libraryPlayer = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/library/video/$id',
  component: PlayerPage,
})
const librarySeries = createRoute({
  getParentRoute: () => libraryRoot,
  path: '/series/$id',
  component: SeriesDetailPage,
})
const libraryNotFound = createRoute({
  getParentRoute: () => libraryRoot,
  path: '*',
  beforeLoad: () => redirect({ to: '/library' }),
  component: () => null,
})
const libraryTree = libraryRoot.addChildren([
  libraryIndex,
  libraryPlayer,
  librarySeries,
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

// Routers are typed with distinct route trees; Router's generics are invariant
// (the `update` method), so unify to a common AnyRouter at this single boundary.
export const routers: Record<TabId, AnyRouter> = {
  home: homeRouter as unknown as AnyRouter,
  library: libraryRouter as unknown as AnyRouter,
  search: searchRouter as unknown as AnyRouter,
  explorer: explorerRouter as unknown as AnyRouter,
}
