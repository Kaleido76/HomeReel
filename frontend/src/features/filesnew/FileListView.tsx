import { useEffect, useRef, useState } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { ArrowUp, Check, Folder, Loader2, RefreshCw, X } from 'lucide-react'
import { ApiError } from '../../api/client'
import type { Fs2Entry } from '../../api/fsbrowse'
import { fileStyle } from './fileType'
import { formatBytes, formatTime } from './path'
import type { SortState } from './Toolbar'
import { useWindowDrag } from './drag'

const COL_WIDTH_KEY = 'filesnew.colWidths.v2'

interface ColWidths {
  name: number
  size: number
  mtime: number
}

type ColKey = keyof ColWidths

// Name is the primary content and gets the bulk of the width; size/mtime only
// need to sit loosely in the remaining space.
const DEFAULT_WIDTHS: ColWidths = { name: 480, size: 150, mtime: 220 }

function loadWidths(): ColWidths {
  try {
    const raw = localStorage.getItem(COL_WIDTH_KEY)
    if (raw) return { ...DEFAULT_WIDTHS, ...JSON.parse(raw) as Partial<ColWidths> }
  } catch {
    // fall back to defaults
  }
  return DEFAULT_WIDTHS
}

// FileListView renders the browse area as a Windows-Explorer-style list: a
// checkbox cell per row (the whole cell toggles selection), name with type icon,
// size and modified time. Column widths are user-resizable via the header drag
// handles and persist in localStorage. Folders always sort first; within each
// group rows follow the active sort key. Clicking a folder opens it, clicking a
// file selects it as a single selection.
export function FileListView({
  path,
  entries,
  loading,
  error,
  selected,
  renaming,
  sort,
  canGoUp,
  onToggle,
  onSelect,
  onNavigate,
  onGoUp,
  onRetry,
  onRenameCancel,
  onRenameCommit,
  emptyText = '空目录',
}: {
  path: string
  entries: Fs2Entry[]
  loading: boolean
  error: ApiError | null
  selected: Set<string>
  renaming: string | null
  sort: SortState
  canGoUp: boolean
  onToggle: (p: string) => void
  onSelect: (p: string) => void
  onNavigate: (p: string) => void
  onGoUp: () => void
  onRetry: () => void
  onRenameCancel: () => void
  onRenameCommit: (p: string, newName: string) => void
  emptyText?: string
}) {
  const [colWidths, setColWidths] = useState<ColWidths>(loadWidths)
  const [renameValue, setRenameValue] = useState('')

  // Prefill the rename input with the entry's current name whenever a row
  // switches into renaming state.
  useEffect(() => {
    if (renaming) {
      const e = entries.find((x) => x.path === renaming)
      if (e) setRenameValue(e.name)
    }
  }, [renaming, entries])

  const dragCol = useRef<{ key: ColKey; startWidth: number } | null>(null)
  const beginDrag = useWindowDrag((dx) => {
    if (!dragCol.current) return
    const { key, startWidth } = dragCol.current
    const min = key === 'name' ? 120 : 80
    setColWidths((prev) => {
      const next = { ...prev, [key]: Math.max(min, startWidth + dx) }
      try {
        localStorage.setItem(COL_WIDTH_KEY, JSON.stringify(next))
      } catch {
        // ignore quota errors
      }
      return next
    })
  })

  function onHeaderDrag(key: ColKey, e: ReactMouseEvent) {
    dragCol.current = { key, startWidth: colWidths[key] }
    beginDrag(e)
  }

  const sorted = [...entries].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    const dir = sort.dir === 'asc' ? 1 : -1
    switch (sort.key) {
      case 'size':
        return (a.size - b.size) * dir
      case 'mtime':
        return (a.mtime - b.mtime) * dir
      default:
        return a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' }) * dir
    }
  })

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center text-neutral-400">
        <Loader2 className="size-5 animate-spin" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-neutral-400">
        <p>{error.message}</p>
        <button
          onClick={onRetry}
          className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50"
        >
          <RefreshCw className="size-4" /> 重试
        </button>
      </div>
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {!path ? (
        <div className="flex h-full items-center justify-center text-neutral-400">
          <div className="text-center">
            <Folder className="mx-auto mb-2 size-10" />
            <p>请从左侧选择盘符或常用目录开始浏览</p>
          </div>
        </div>
      ) : (
        <div className="flex min-w-fit flex-col">
          <div className="sticky top-0 z-10 flex items-stretch border-b border-neutral-200 bg-neutral-50 text-left text-xs font-medium text-neutral-500">
            <div className="w-10 shrink-0 px-1 py-2.5" />
            <ColumnHeader title="名称" width={colWidths.name} onDrag={(e) => onHeaderDrag('name', e)} />
            <ColumnHeader title="大小" width={colWidths.size} className="hidden md:flex" onDrag={(e) => onHeaderDrag('size', e)} />
            <ColumnHeader title="修改时间" width={colWidths.mtime} className="hidden lg:flex" onDrag={(e) => onHeaderDrag('mtime', e)} />
          </div>

          {canGoUp && (
            <div
              onClick={onGoUp}
              className="flex cursor-pointer items-stretch border-b border-neutral-50 hover:bg-neutral-100"
            >
              <div className="w-10 shrink-0" />
              <div className="flex flex-1 items-center gap-2 px-4 py-2.5 text-neutral-500">
                <ArrowUp className="size-4" /> ..
              </div>
            </div>
          )}

          {sorted.map((e) => {
            const isRenaming = renaming === e.path
            const isSelected = selected.has(e.path)
            const typeIcon = e.is_dir ? (
              <Folder className="size-4 shrink-0 text-neutral-400" />
            ) : (() => {
              const style = fileStyle(e.name)
              return <style.icon className={`size-4 shrink-0 ${style.className}`} />
            })()
            return (
              <div
                key={e.path}
                onClick={() => {
                  if (isRenaming) return
                  if (e.is_dir) onNavigate(e.path)
                  else onSelect(e.path)
                }}
                className={`flex cursor-pointer items-stretch border-b border-neutral-50 transition-colors hover:bg-neutral-100 ${
                  isSelected ? 'bg-blue-50/60' : ''
                }`}
              >
                <div
                  onClick={(ev) => {
                    ev.stopPropagation()
                    onToggle(e.path)
                  }}
                  title="选择"
                  className="flex w-10 shrink-0 items-center justify-center px-1 py-2.5"
                >
                  <input
                    type="checkbox"
                    checked={isSelected}
                    onChange={() => onToggle(e.path)}
                    onClick={(ev) => ev.stopPropagation()}
                    className="accent-blue-600"
                  />
                </div>
                <div style={{ width: colWidths.name }} className="flex shrink-0 items-center gap-2 px-4 py-2.5">
                  {isRenaming ? (
                    <RenameForm
                      value={renameValue}
                      onChange={setRenameValue}
                      onCancel={onRenameCancel}
                      onSubmit={() => onRenameCommit(e.path, renameValue)}
                    />
                  ) : (
                    <span className={`flex min-w-0 items-center gap-2 ${isSelected ? 'text-blue-700' : 'text-neutral-800'}`}>
                      {typeIcon}
                      <span className="truncate">{e.name}</span>
                    </span>
                  )}
                </div>
                <div style={{ width: colWidths.size }} className="hidden shrink-0 items-center px-4 py-2.5 text-neutral-500 md:flex">
                  {e.is_dir ? '—' : formatBytes(e.size)}
                </div>
                <div style={{ width: colWidths.mtime }} className="hidden shrink-0 items-center px-4 py-2.5 text-neutral-500 lg:flex">
                  {formatTime(e.mtime)}
                </div>
              </div>
            )
          })}

          {sorted.length === 0 && (
            <div className="px-4 py-10 text-center text-neutral-400">{emptyText}</div>
          )}
        </div>
      )}
    </div>
  )
}

function ColumnHeader({
  title,
  width,
  className,
  onDrag,
}: {
  title: string
  width: number
  className?: string
  onDrag: (e: ReactMouseEvent) => void
}) {
  return (
    <div style={{ width }} className={`group relative shrink-0 items-center px-4 py-2.5 ${className ?? ''}`}>
      <span className="truncate">{title}</span>
      <div
        onMouseDown={(e) => {
          e.stopPropagation()
          onDrag(e)
        }}
        title="拖动调整宽度"
        className="absolute inset-y-0 right-0 w-1.5 cursor-col-resize group-hover:bg-neutral-300"
      />
    </div>
  )
}

function RenameForm({
  value,
  onChange,
  onCancel,
  onSubmit,
}: {
  value: string
  onChange: (v: string) => void
  onCancel: () => void
  onSubmit: () => void
}) {
  return (
    <form
      onSubmit={(ev) => {
        ev.preventDefault()
        if (value.trim()) onSubmit()
      }}
      className="flex items-center gap-1"
    >
      <input
        value={value}
        onChange={(ev) => onChange(ev.target.value)}
        autoFocus
        onFocus={(ev) => ev.target.select()}
        className="rounded-md border border-neutral-300 px-1.5 py-0.5 text-sm outline-none focus:border-blue-600"
      />
      <button type="submit" disabled={!value.trim()} title="确认" className="rounded p-1 text-emerald-600 hover:bg-emerald-50">
        <Check className="size-4" />
      </button>
      <button type="button" onClick={onCancel} title="取消" className="rounded p-1 text-neutral-400 hover:bg-neutral-100">
        <X className="size-4" />
      </button>
    </form>
  )
}
