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
      className="flex min-w-0 items-center gap-1 overflow-x-auto"
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
            className={`flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              active
                ? 'bg-neutral-900 text-white'
                : 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900'
            }`}
          >
            <def.icon className="size-4" />
            <span className="hidden sm:inline">{def.label}</span>
          </button>
        )
      })}
    </div>
  )
}
