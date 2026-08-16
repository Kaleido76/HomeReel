import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Filter, RotateCcw } from 'lucide-react'
import { fetchTags } from '../../api/videos'
import { emptyFilters, type GridState } from './types'

// Draft is the editable copy of the advanced filters while the panel is open.
// Nothing is applied until the toggle button is clicked again — the same
// JobsIndicator-style toggle: first click expands, second click applies and
// collapses. Only tags remain as a filter (genre/year/sort were removed).
type Draft = {
  tags: string[]
}

function toDraft(f: Pick<GridState, 'tags'>): Draft {
  return { tags: f.tags }
}

export type AppliedAdvancedFilters = Pick<GridState, 'tags'>

export function AdvancedFilter({
  filters,
  onApply,
}: {
  filters: AppliedAdvancedFilters
  onApply: (f: AppliedAdvancedFilters) => void
}) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(() => toDraft(filters))

  const tagsQuery = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    enabled: open,
  })

  const activeCount = filters.tags.length

  function toggle() {
    if (open) {
      onApply(draft)
      setOpen(false)
    } else {
      setDraft(toDraft(filters))
      setOpen(true)
    }
  }

  function toggleTag(tag: string) {
    setDraft((d) => ({
      ...d,
      tags: d.tags.includes(tag) ? d.tags.filter((t) => t !== tag) : [...d.tags, tag],
    }))
  }

  function reset() {
    setDraft({ ...emptyFilters })
  }

  return (
    <div className="relative">
      <button
        onClick={toggle}
        aria-expanded={open}
        aria-haspopup="dialog"
        className={`flex h-[34px] shrink-0 items-center gap-1.5 rounded border px-2.5 text-sm transition-colors ${
          activeCount > 0
            ? 'border-blue-500 bg-blue-50 text-blue-700'
            : 'border-neutral-300 bg-white text-neutral-700 hover:border-blue-400 hover:text-blue-600'
        }`}
      >
        <Filter className="size-4" />
        <span className="hidden sm:inline">高级筛选</span>
        {activeCount > 0 && (
          <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-600 px-1 text-[10px] font-medium leading-none text-white">
            {activeCount}
          </span>
        )}
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="高级筛选"
          className="absolute right-0 top-full z-50 mt-1 w-72 overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-lg"
        >
          <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2">
            <p className="text-sm font-medium text-neutral-900">高级筛选</p>
            <button
              onClick={reset}
              className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
            >
              <RotateCcw className="size-3" /> 重置
            </button>
          </div>
          <div className="max-h-[70vh] space-y-3 overflow-y-auto p-3">
            <Field label="标签">
              {tagsQuery.isLoading ? (
                <p className="text-xs text-neutral-400">加载中…</p>
              ) : (
                <div className="flex flex-wrap gap-1">
                  {(tagsQuery.data?.tags ?? []).map((t) => {
                    const on = draft.tags.includes(t.tag)
                    return (
                      <button
                        key={t.tag}
                        onClick={() => toggleTag(t.tag)}
                        className={`rounded-full border px-2 py-0.5 text-xs transition-colors ${
                          on
                            ? 'border-blue-600 bg-blue-600 text-white'
                            : 'border-neutral-200 bg-neutral-50 text-neutral-600 hover:border-blue-400 hover:text-blue-600'
                        }`}
                      >
                        {t.tag}
                      </button>
                    )
                  })}
                  {draft.tags.length === 0 && (tagsQuery.data?.tags.length ?? 0) === 0 && (
                    <p className="text-xs text-neutral-400">暂无标签</p>
                  )}
                </div>
              )}
            </Field>
          </div>
        </div>
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-neutral-500">{label}</span>
      {children}
    </label>
  )
}
