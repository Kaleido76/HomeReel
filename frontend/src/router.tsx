import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './components/AppShell'
import { ExplorerPage } from './features/explorer/ExplorerPage'
import { HomePage } from './features/home/HomePage'
import { LibraryPage } from './features/library/LibraryPage'
import { PlayerPage } from './features/player/PlayerPage'

const rootRoute = createRootRoute({
  component: AppShell,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: HomePage,
})

const libraryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/library',
  component: LibraryPage,
})

const playerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/library/video/$id',
  component: PlayerPage,
})

const explorerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/explorer',
  component: ExplorerPage,
  validateSearch: (search) => ({
    storageId: typeof search.storageId === 'string' ? search.storageId : '',
    path: typeof search.path === 'string' ? search.path : '',
  }),
})

const notFoundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '*',
  beforeLoad: () => redirect({ to: '/' }),
  component: () => null,
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  libraryRoute,
  playerRoute,
  explorerRoute,
  notFoundRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
