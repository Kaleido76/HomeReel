import { Suspense, useEffect, useSyncExternalStore, useState } from 'react'
import { RouterProvider } from '@tanstack/react-router'
import { Home } from 'lucide-react'
import { TAB_DEFS, TAB_ROOTS, type TabId } from './config'
import { routers } from './routers'
import { backToHome, getActiveTab, subscribeTabs } from './manager'
import { TabSync } from './TabSync'

function PaneLoader() {
  return (
    <div className="flex h-64 items-center justify-center text-neutral-400">
      <div className="size-6 animate-spin rounded-full border-2 border-neutral-300 border-t-indigo-600" />
    </div>
  )
}

// TabHomeButton returns the tab to its root view; shown only when the tab is on
// a deep page (player, series detail) so users can always get back "home".
function TabHomeButton({ tab }: { tab: TabId }) {
  const pathname = useSyncExternalStore(
    (cb) => routers[tab].subscribe('onResolved', cb),
    () => routers[tab].state.location.pathname,
  )
  if (pathname === TAB_ROOTS[tab]) return null
  const def = TAB_DEFS.find((d) => d.id === tab)!
  return (
    <button
      onClick={() => backToHome(tab)}
      className="mb-4 flex items-center gap-1.5 rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 transition-colors hover:bg-neutral-50 hover:text-neutral-900"
    >
      <Home className="size-4" /> 回到{def.label}首页
    </button>
  )
}

// TabHost renders every mounted tab router in its own scroll pane. Tabs stay
// mounted once visited (keep-alive); only the active pane is visible. Explorer
// manages its own two-column internal scrolling, the other panes scroll as one.
export function TabHost() {
  const activeTab = useSyncExternalStore(subscribeTabs, getActiveTab)
  const [mounted, setMounted] = useState<Set<TabId>>(() => new Set([activeTab]))

  useEffect(() => {
    if (!mounted.has(activeTab)) {
      setMounted((prev) => new Set([...prev, activeTab]))
    }
  }, [activeTab, mounted])

  return (
    <div className="h-full">
      <TabSync />
      {TAB_DEFS.map((def) => {
        if (!mounted.has(def.id)) return null
        const explorer = def.id === 'explorer'
        return (
          <div
            key={def.id}
            id={`panel-${def.id}`}
            role="tabpanel"
            aria-labelledby={`tab-${def.id}`}
            hidden={def.id !== activeTab}
            className={`h-full ${explorer ? '' : 'overflow-y-auto'}`}
          >
            <div className={`mx-auto w-full max-w-6xl ${explorer ? 'h-full px-4 py-6' : 'px-4 py-6'}`}>
              {!explorer && <TabHomeButton tab={def.id} />}
              <Suspense fallback={<PaneLoader />}>
                <RouterProvider router={routers[def.id]} />
              </Suspense>
            </div>
          </div>
        )
      })}
    </div>
  )
}
