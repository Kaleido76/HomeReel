import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { Wrench } from 'lucide-react'
import { TOOL_DEFS } from './tools'

interface ToolsSearch {
  tool?: string
}

// ToolsPage is the 工具 tab (like the file browser's rail+panel layout): a
// full-height narrow left rail lists the available tools and the right pane
// hosts the selected tool. Tools stay mounted once visited (display:none when
// hidden) so each keeps its view state across switches, mirroring the tab
// host's keep-alive. The active tool lives in the URL (?tool=id).
export function ToolsPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { tool } = (location.search ?? {}) as ToolsSearch

  const activeId = TOOL_DEFS.some((t) => t.id === tool) ? (tool as string) : TOOL_DEFS[0].id
  const [mounted, setMounted] = useState<Set<string>>(() => new Set([activeId]))
  useEffect(() => {
    if (!mounted.has(activeId)) {
      setMounted((prev) => new Set([...prev, activeId]))
    }
  }, [activeId, mounted])

  return (
    <div className="flex h-full min-h-0">
      <aside className="flex h-full w-56 shrink-0 flex-col border-r border-neutral-200 bg-white">
        <p className="flex shrink-0 items-center gap-2 border-b border-neutral-200 px-2.5 py-2 text-xs font-medium uppercase tracking-wide text-neutral-500">
          <Wrench className="size-3.5" /> 工具
        </p>
        <nav className="min-h-0 flex-1 overflow-y-auto" aria-label="工具列表">
          {TOOL_DEFS.map((def) => {
            const active = def.id === activeId
            const Icon = def.icon
            return (
              <button
                key={def.id}
                onClick={() => navigate({ to: '/tools', search: { tool: def.id } })}
                aria-current={active ? 'page' : undefined}
                className={`flex w-full items-center gap-2.5 px-2.5 py-2 text-left text-sm transition-colors ${
                  active
                    ? 'bg-neutral-900 text-white'
                    : 'text-neutral-700 hover:bg-neutral-100 hover:text-neutral-900'
                }`}
              >
                <Icon className={`size-5 shrink-0 ${active ? 'text-white' : 'text-neutral-400'}`} />
                <span className="min-w-0 flex-1 truncate font-medium">{def.label}</span>
              </button>
            )
          })}
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
