import { useRef, useSyncExternalStore, type KeyboardEvent } from 'react'
import { TAB_DEFS } from './config'
import { activate, getActiveTab, subscribeTabs } from './manager'

// TabBar is the top-level "browser tab" strip. It uses the ARIA tablist pattern
// (role=tab/tablist, arrow-key navigation) and shows a clear active indicator.
export function TabBar() {
  const activeTab = useSyncExternalStore(subscribeTabs, getActiveTab)
  const listRef = useRef<HTMLDivElement>(null)

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const idx = TAB_DEFS.findIndex((d) => d.id === activeTab)
    let next = -1
    if (e.key === 'ArrowRight') next = (idx + 1) % TAB_DEFS.length
    else if (e.key === 'ArrowLeft') next = (idx - 1 + TAB_DEFS.length) % TAB_DEFS.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = TAB_DEFS.length - 1
    if (next !== -1) {
      e.preventDefault()
      const el = listRef.current?.children[next] as HTMLElement | undefined
      el?.focus()
      activate(TAB_DEFS[next].id)
    }
  }

  return (
    <div
      ref={listRef}
      role="tablist"
      aria-label="主导航"
      onKeyDown={onKeyDown}
      className="flex min-w-0 items-stretch gap-1 overflow-x-auto"
    >
      {TAB_DEFS.map((def) => {
        const active = def.id === activeTab
        return (
          <button
            key={def.id}
            role="tab"
            id={`tab-${def.id}`}
            aria-selected={active}
            aria-controls={`panel-${def.id}`}
            tabIndex={active ? 0 : -1}
            onClick={() => activate(def.id)}
            className={`relative flex h-full shrink-0 items-center gap-1.5 px-3 text-sm transition-colors ${
              active ? 'text-neutral-900' : 'text-neutral-500 hover:text-neutral-800'
            }`}
          >
            <def.icon className="size-4" />
            {def.label}
            <span
              aria-hidden="true"
              className={`absolute inset-x-2 bottom-0 h-0.5 transition-colors ${
                active ? 'bg-blue-600' : 'bg-transparent'
              }`}
            />
          </button>
        )
      })}
    </div>
  )
}
