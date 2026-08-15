import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, Film, Search } from 'lucide-react'
import { Modal } from './Modal'
import { fetchSeries, type Series } from '../api/series'

// SeriesPickerModal is the system-wide series picker (Windows file-picker style):
// a filter input at the top (case-insensitive substring match on the name), a
// scrollable list of all library series below (each row shows name + member
// count), and a confirm bar. Selection mode is fixed by the caller: `multiple`
// renders checkboxes and toggles rows, `single` renders a radio-style indicator
// and picking a row replaces the previous selection. `initialSelectedIds` pre-checks
// rows on open (e.g. already-linked series in a manage dialog). Confirming returns
// the selected Series[] (empty list if none) — the caller owns what happens next,
// including the diff against previous selection; canceling closes without a result.
// `excludeIds` hides series that must not be pickable (e.g. the current series).
export function SeriesPickerModal({
  multiple,
  excludeIds = [],
  initialSelectedIds = [],
  title = '选择系列',
  titleIcon,
  onConfirm,
  onClose,
}: {
  multiple: boolean
  excludeIds?: string[]
  initialSelectedIds?: string[]
  title?: string
  titleIcon?: React.ReactNode
  onConfirm: (selected: Series[]) => void
  onClose: () => void
}) {
  const query = useQuery({ queryKey: ['series'], queryFn: () => fetchSeries() })
  const [filter, setFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set(initialSelectedIds))

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    const rows = (query.data?.series ?? []).filter((s) => !excludeIds.includes(s.id))
    if (!q) return rows
    return rows.filter((s) => s.name.toLowerCase().includes(q))
  }, [query.data, filter, excludeIds])

  const selected = (query.data?.series ?? []).filter((s) => selectedIds.has(s.id))

  function toggle(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (multiple) {
        if (next.has(id)) next.delete(id)
        else next.add(id)
      } else {
        if (next.has(id)) next.delete(id)
        else {
          next.clear()
          next.add(id)
        }
      }
      return next
    })
  }

  function confirm() {
    onConfirm(selected)
    onClose()
  }

  return (
    <Modal onClose={onClose} size="md" title={title} titleIcon={titleIcon ?? <Film className="size-4 text-neutral-500" />}>
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-100 px-3 py-2">
        <Search className="size-3.5 shrink-0 text-neutral-400" />
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="筛选系列…"
          spellCheck={false}
          autoFocus
          onKeyDown={(e) => {
            if (e.key === 'Enter' && selected.length > 0) confirm()
          }}
          className="w-full rounded-md border border-neutral-300 px-2 py-1 text-xs outline-none focus:border-blue-600"
        />
        <span className="shrink-0 text-[11px] text-neutral-400">{filtered.length} 项</span>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {query.isLoading && <p className="px-3 py-6 text-center text-xs text-neutral-400">加载中…</p>}
        {!query.isLoading && filtered.length === 0 && (
          <p className="px-3 py-6 text-center text-xs text-neutral-400">没有匹配的系列。</p>
        )}
        {filtered.map((s) => {
          const checked = selectedIds.has(s.id)
          return (
            <button
              key={s.id}
              onClick={() => toggle(s.id)}
              className={`flex w-full items-center gap-2.5 border-b border-neutral-50 px-3 py-2 text-left transition-colors hover:bg-blue-50/40 ${
                checked ? 'bg-blue-50/60' : ''
              }`}
            >
              {multiple ? (
                <span
                  className={`flex size-4 shrink-0 items-center justify-center rounded border ${
                    checked ? 'border-blue-600 bg-blue-600 text-white' : 'border-neutral-300 bg-white'
                  }`}
                >
                  {checked && <Check className="size-3" />}
                </span>
              ) : (
                <span
                  className={`size-3.5 shrink-0 rounded-full border ${
                    checked ? 'border-blue-600 bg-blue-600' : 'border-neutral-300 bg-white'
                  }`}
                />
              )}
              <span className="min-w-0 flex-1 truncate text-sm text-neutral-800" title={s.name}>
                {s.name}
              </span>
              <span className="shrink-0 text-xs text-neutral-400">{s.member_count} 集</span>
            </button>
          )
        })}
      </div>

      <div className="flex shrink-0 items-center justify-between gap-2 border-t border-neutral-100 bg-neutral-50 px-3 py-2">
        <span className="truncate text-[11px] text-neutral-500">
          {selected.length === 0 ? '未选择系列' : `已选择 ${selected.length} 个系列`}
        </span>
        <div className="flex shrink-0 gap-2">
          <button
            onClick={onClose}
            className="rounded-lg border border-neutral-200 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-100"
          >
            取消
          </button>
          <button
            onClick={confirm}
            disabled={selected.length === 0}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Check className="size-4" /> 确定
          </button>
        </div>
      </div>
    </Modal>
  )
}
