import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import { AppShell } from './components/AppShell'
import { ExplorerPage } from './features/explorer/ExplorerPage'
import { HomePage } from './features/home/HomePage'

const rootRoute = createRootRoute({
  component: AppShell,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: HomePage,
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

const routeTree = rootRoute.addChildren([indexRoute, explorerRoute, notFoundRoute])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
