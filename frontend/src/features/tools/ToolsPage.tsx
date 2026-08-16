import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { ChevronDown, ChevronUp, Wrench } from 'lucide-react'
import { TOOL_DEFS } from './tools'

interface ToolsSearch {
  tool?: string
}

// ToolsPage is the 工具 tab. On wide screens (>=lg) it is the rail+panel split:
// a narrow left rail lists the available tools and the right pane hosts the
// selected tool. On narrow screens the rail collapses into a full-width context
// row below the global nav showing the current tool; tapping it expands a
// full-width dropdown drawer downward (fade + slight slide, overlay — does not
// push content). Together the row and the drawer form one list: the row is the
// current-tool item (blue + bold), the drawer lists the other tools. The
// backdrop dims only the tools area, leaving the global header untouched. Tools
// stay mounted once visited (display:none when hidden) so each keeps its view
// state across switches. The active tool lives in the URL (?tool=id).
export function ToolsPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { tool } = (location.search ?? {}) as ToolsSearch

  const activeDef = TOOL_DEFS.find((t) => t.id === tool) ?? TOOL_DEFS[0]
  const activeId = activeDef.id
  const [mounted, setMounted] = useState<Set<string>>(() => new Set([activeId]))
  useEffect(() => {
    if (!mounted.has(activeId)) {
      setMounted((prev) => new Set([...prev, activeId]))
    }
  }, [activeId, mounted])

  const [menuOpen, setMenuOpen] = useState(false)
  // The narrow-screen tool drawer closes on Escape.
  useEffect(() => {
    if (!menuOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [menuOpen])

  function selectTool(id: string) {
    setMenuOpen(false)
    navigate({ to: '/tools', search: { tool: id } })
  }

  const ActiveIcon = activeDef.icon

  return (
    <div className="relative flex h-full min-h-0 flex-col lg:flex-row">
      {/* Narrow screens: a full-width context row showing the current tool.
          Tapping it expands the drawer; the row is the "current tool" item of
          the list (blue + bold) and the drawer holds the other tools. */}
      <div className="relative z-50 flex shrink-0 items-center border-b border-neutral-200 bg-white lg:hidden">
        <button
          onClick={() => setMenuOpen((o) => !o)}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm"
        >
          <ActiveIcon className="size-4 shrink-0 text-blue-600" />
          <span
            className={`min-w-0 truncate ${
              menuOpen ? 'font-semibold text-blue-700' : 'font-medium text-neutral-800'
            }`}
          >
            {activeDef.label}
          </span>
          {menuOpen ? (
            <ChevronUp className="ml-auto size-4 shrink-0 text-blue-500" />
          ) : (
            <ChevronDown className="ml-auto size-4 shrink-0 text-neutral-400" />
          )}
        </button>

        {/* The drawer is always mounted so opacity/transform can animate both
            the fade-in and the fade-out; it is pointer-inert while closed. */}
        <div
          inert={!menuOpen}
          className={`absolute inset-x-0 top-full z-50 overflow-hidden rounded-b-lg shadow-lg transition-[opacity,transform] duration-150 ${
            menuOpen ? 'translate-y-0 opacity-100' : 'pointer-events-none -translate-y-1 opacity-0'
          }`}
        >
          <div className="max-h-[60vh] divide-y divide-neutral-100 overflow-y-auto border-b border-neutral-200 border-t border-neutral-100 bg-white py-1">
            <ToolList activeId={activeId} onSelect={selectTool} variant="menu" excludeActive />
          </div>
        </div>
      </div>

      {/* Backdrop dims only the tools area (anchored to this container, which
          starts below the global header), never the header itself. */}
      <div
        onClick={() => setMenuOpen(false)}
        className={`absolute inset-0 z-40 bg-black/25 transition-opacity duration-150 lg:hidden ${
          menuOpen ? 'opacity-100' : 'pointer-events-none opacity-0'
        }`}
      />

      {/* Wide screens: the persistent left rail. */}
      <aside className="hidden h-full w-56 shrink-0 border-r border-neutral-200 bg-white p-2 lg:block">
        <p className="flex items-center gap-1.5 px-1.5 pb-2 pt-1 text-xs font-medium uppercase tracking-wide text-neutral-400">
          <Wrench className="size-3.5" /> 工具
        </p>
        <nav className="flex flex-col gap-0.5" aria-label="工具列表">
          <ToolList activeId={activeId} onSelect={selectTool} variant="rail" />
        </nav>
      </aside>

      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-neutral-50">
        {TOOL_DEFS.map((def) => {
          if (!mounted.has(def.id)) return null
          const Comp = def.component
          return (
            <div key={def.id} hidden={def.id !== activeId} className="min-h-full px-4 py-6 sm:px-6 xl:px-8">
              <Comp />
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ToolList renders the tool rows shared by the desktop rail and the narrow
// drawer. `variant` switches between the compact rail look (rounded buttons,
// solid blue active) and the thumb-friendly drawer rows (larger padding, soft
// blue active), mirroring the library view selector's two screen sizes.
// `excludeActive` drops the current tool from the list — used by the drawer,
// where the context row above already stands in for the current tool.
function ToolList({
  activeId,
  onSelect,
  variant,
  excludeActive = false,
}: {
  activeId: string
  onSelect: (id: string) => void
  variant: 'rail' | 'menu'
  excludeActive?: boolean
}) {
  const isRail = variant === 'rail'
  return TOOL_DEFS.map((def) => {
    const active = def.id === activeId
    if (excludeActive && active) return null
    const Icon = def.icon
    return (
      <button
        key={def.id}
        onClick={() => onSelect(def.id)}
        aria-current={active ? 'page' : undefined}
        className={`flex w-full items-center gap-2.5 text-left transition-colors ${
          isRail
            ? active
              ? 'rounded-md bg-blue-600 px-2.5 py-2 text-sm font-medium text-white'
              : 'rounded-md px-2.5 py-2 text-sm text-neutral-700 hover:bg-neutral-100'
            : active
              ? 'px-4 py-4 text-sm font-medium text-blue-700 hover:bg-blue-50'
              : 'px-4 py-4 text-sm text-neutral-600 hover:bg-neutral-50'
        }`}
      >
        <Icon
          className={`size-4 shrink-0 ${active ? (isRail ? 'text-white' : 'text-blue-700') : 'text-neutral-400'}`}
        />
        <span className="min-w-0 flex-1 truncate font-medium">{def.label}</span>
      </button>
    )
  })
}
