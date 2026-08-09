import { Suspense, useEffect, useSyncExternalStore, useState } from 'react'
import { RouterProvider } from '@tanstack/react-router'
import { TAB_DEFS, type TabId } from './config'
import { routers } from './routers'
import { getActiveTab, subscribeTabs } from './manager'
import { TabSync } from './TabSync'

function PaneLoader() {
  return (
    <div className="flex h-64 items-center justify-center text-neutral-400">
      <div className="size-6 animate-spin rounded-full border-2 border-neutral-300 border-t-blue-600" />
    </div>
  )
}

// Panes that manage their own full-height internal scrolling (library uses a
// grid + right drawer split, the file browser and the tools rail manage their
// own panes) opt out of the shared scroll wrapper so nested scroll regions work
// on wide screens.
const selfScrolling: ReadonlySet<TabId> = new Set(['library', 'files', 'tools'])

// Tabs that render a full-bleed layout (no max-width / padding) are exempt from
// the shared padded wrapper, exactly like the library panel.
const fullBleed: ReadonlySet<TabId> = new Set(['library', 'files', 'tools'])

// TabHost renders every mounted tab router in its own pane. Tabs stay mounted
// once visited (keep-alive); only the active pane is visible. Home/search use
// the shared page-level scroll; library/explorer manage their own layout.
//
// The library tab is full-bleed on wide screens: its three-column layout is
// sized in viewport units (50vw per column) so it must not be capped by a
// max-width or padded — side whitespace would defeat the panel-style UI.
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
        const inner = selfScrolling.has(def.id) ? 'h-full' : 'h-full overflow-y-auto'
        return (
          <div
            key={def.id}
            id={`panel-${def.id}`}
            role="tabpanel"
            aria-labelledby={`tab-${def.id}`}
            hidden={def.id !== activeTab}
            className={inner}
          >
            <div
              className={
                fullBleed.has(def.id)
                  ? 'h-full w-full'
                  : `mx-auto w-full px-4 sm:px-6 xl:px-8 ${
                      selfScrolling.has(def.id)
                        ? 'h-full max-w-[1920px]'
                        : 'max-w-[1600px] py-6'
                    }`
              }
            >
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
