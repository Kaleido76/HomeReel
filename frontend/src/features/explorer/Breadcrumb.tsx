import { ChevronRight } from 'lucide-react'
import type { Storage } from '../../api/storages'

export function Breadcrumb({
  storage,
  path,
  onNavigate,
}: {
  storage?: Storage
  path: string
  onNavigate: (path: string) => void
}) {
  const segments = path ? path.split('/') : []
  return (
    <div className="flex items-center gap-1 text-sm text-neutral-600">
      {storage && <span className="font-medium text-neutral-900">{storage.name}</span>}
      {segments.map((seg, i) => {
        const p = segments.slice(0, i + 1).join('/')
        return (
          <span key={p} className="flex items-center gap-1">
            <ChevronRight className="size-3.5 text-neutral-400" />
            <button
              onClick={() => onNavigate(p)}
              className="rounded px-1 py-0.5 hover:bg-neutral-100 hover:text-neutral-900"
            >
              {seg}
            </button>
          </span>
        )
      })}
    </div>
  )
}
