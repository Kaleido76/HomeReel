import { useEffect, useRef, useState } from 'react'
import { ArrowDownUp, ArrowUp, ChevronRight, ClipboardCopy, ClipboardPaste, Computer, Factory, Film, Layers, ListChecks, MoreHorizontal, Pencil, Pin, ReplaceAll, Scissors, Trash2, Video } from 'lucide-react'
import { pathSegments } from './path'
import { Tooltip } from '../../components/Tooltip'

export type SortKey = 'name' | 'size' | 'mtime'

export interface SortState {
  key: SortKey
  dir: 'asc' | 'desc'
}

export type ClipMode = 'copy' | 'cut'

// ClipboardItem captures the display metadata at cut/copy time so the tool
// drawer can still render name/icon after navigating away from the folder.
export interface ClipboardItem {
  path: string
  name: string
  is_dir: boolean
}

export interface Clipboard {
  mode: ClipMode
  items: ClipboardItem[]
}

const btnCls =
  'flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40'

// Toolbar has two rows like a Windows Explorer toolbar: the top is a full-width
// breadcrumb of the current path (every segment jumps to that ancestor), the
// bottom holds the clipboard actions (enabled once rows are checked) plus the
// view/sort controls. On narrow screens (<1024px) the action row collapses to
// a compact set with a "More" overflow menu for less frequent actions.
export function Toolbar({
  path,
  canGoUp,
  entryCount,
  selectedCount,
  clipboard,
  notice,
  onNavigate,
  onGoUp,
  onCut,
  onCopy,
  onPaste,
  onRename,
  onDelete,
  onSelectAll,
  onInvertSelection,
  onBatchRename,
  onMarkSeries,
  canMarkSeries,
  onFormat,
  canFormat,
  onPin,
  pinned,
  isSource,
  onToggleSource,
  mediaOnly,
  onToggleMedia,
  wide,
}: {
  path: string
  canGoUp: boolean
  entryCount: number
  selectedCount: number
  clipboard: Clipboard | null
  notice: string
  onNavigate: (p: string) => void
  onGoUp: () => void
  onCut: () => void
  onCopy: () => void
  onPaste: () => void
  onRename: () => void
  onDelete: () => void
  onSelectAll: () => void
  onInvertSelection: () => void
  onBatchRename: () => void
  onMarkSeries: () => void
  canMarkSeries: boolean
  onFormat: () => void
  canFormat: boolean
  onPin: () => void
  pinned: boolean
  isSource: boolean
  onToggleSource: () => void
  mediaOnly: boolean
  onToggleMedia: () => void
  wide: boolean
}) {
  const crumbs = pathSegments(path)
  const [moreOpen, setMoreOpen] = useState(false)
  const moreRef = useRef<HTMLDivElement | null>(null)

  const canMutate = selectedCount > 0
  const canPaste = !!path && !!clipboard

  useEffect(() => {
    if (!moreOpen) return
    const onClick = (e: PointerEvent) => {
      if (moreRef.current && !moreRef.current.contains(e.target as Node)) setMoreOpen(false)
    }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setMoreOpen(false) }
    document.addEventListener('pointerdown', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [moreOpen])

  return (
    <div className="shrink-0 border-b border-neutral-200 bg-white">
      {/* Breadcrumb row: full-width, every ancestor is clickable */}
      <div className="flex items-center gap-0.5 overflow-x-auto border-b border-neutral-100 px-3 py-1.5">
        <Tooltip content="这台电脑">
          <button
            onClick={() => onNavigate('')}
            className="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
          >
            <Computer className="size-4" />
          </button>
        </Tooltip>
        {crumbs.map((c, i) => (
          <span key={c.path} className="flex shrink-0 items-center gap-0.5">
            {i > 0 && <ChevronRight className="size-3.5 text-neutral-300" />}
            <button
              onClick={() => onNavigate(c.path)}
              title={c.path}
              className={`max-w-48 truncate rounded-md px-1.5 py-0.5 text-sm transition-colors ${
                c.path === path
                  ? 'bg-neutral-100 text-neutral-900'
                  : 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900'
              }`}
            >
              {c.label}
            </button>
          </span>
        ))}
        {crumbs.length === 0 && <span className="px-1 text-sm text-neutral-400">这台电脑</span>}
      </div>

      {wide ? (
        /* Wide screens: full action row */
        <div className="flex flex-wrap items-center gap-2 px-3 py-2">
          <Tooltip content="上级目录">
            <button
              onClick={onGoUp}
              disabled={!canGoUp}
              className="flex items-center rounded-lg border border-neutral-200 px-3.5 py-2 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ArrowUp className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content={pinned ? '取消固定' : '固定此目录'}>
            <button
              onClick={onPin}
              disabled={!path}
              className={`flex items-center rounded-lg border px-2.5 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 ${
                pinned
                  ? 'border-blue-600 bg-blue-50 text-blue-700'
                  : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
              }`}
            >
              <Pin className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content={isSource ? '取消多媒体源标记' : '标记为多媒体源'}>
            <button
              onClick={onToggleSource}
              disabled={!path}
              className={`flex items-center rounded-lg border px-2.5 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 ${
                isSource
                  ? 'border-violet-600 bg-violet-50 text-violet-700'
                  : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
              }`}
            >
              <Video className="size-4" />
            </button>
          </Tooltip>

          <span className="mx-1 h-5 w-px shrink-0 bg-neutral-200" />

          <Tooltip content="剪切">
            <button onClick={onCut} disabled={!canMutate} className={btnCls}>
              <Scissors className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="复制">
            <button onClick={onCopy} disabled={!canMutate} className={btnCls}>
              <ClipboardCopy className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="粘贴到当前目录">
            <button onClick={onPaste} disabled={!canPaste} className={btnCls}>
              <ClipboardPaste className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="重命名">
            <button onClick={onRename} disabled={selectedCount !== 1} className={btnCls}>
              <Pencil className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="批量重命名">
            <button onClick={onBatchRename} disabled={selectedCount < 2} className={btnCls}>
              <ReplaceAll className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="永久删除">
            <button
              onClick={onDelete}
              disabled={!canMutate}
              className="flex items-center rounded-lg border border-red-200 px-2.5 py-1.5 text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Trash2 className="size-4" />
            </button>
          </Tooltip>

          <Tooltip content="全选">
            <button onClick={onSelectAll} disabled={entryCount === 0} className={btnCls}>
              <ListChecks className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="反选">
            <button onClick={onInvertSelection} disabled={entryCount === 0} className={btnCls}>
              <ArrowDownUp className="size-4" />
            </button>
          </Tooltip>

          <span className="mx-1 h-5 w-px shrink-0 bg-neutral-200" />

          <Tooltip content="标记为系列">
            <button onClick={onMarkSeries} disabled={!canMarkSeries} className={btnCls}>
              <Layers className="size-4" />
            </button>
          </Tooltip>

          <Tooltip content="转到格式工厂">
            <button
              onClick={onFormat}
              disabled={!canFormat}
              className="flex items-center gap-1.5 rounded-lg border border-blue-200 px-2.5 py-1.5 text-sm text-blue-700 hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Factory className="size-4" />
              格式工厂
            </button>
          </Tooltip>

          <Tooltip content="多媒体视图">
            <button
              onClick={onToggleMedia}
              disabled={!path}
              className={`flex items-center rounded-lg border px-2.5 py-1.5 disabled:cursor-not-allowed disabled:opacity-40 ${
                mediaOnly
                  ? 'border-blue-600 bg-blue-50 text-blue-700'
                  : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
              }`}
            >
              <Film className="size-4" />
            </button>
          </Tooltip>

          <span className="ml-auto shrink-0 text-xs text-neutral-400">
            {notice || `${entryCount} 项${selectedCount > 0 ? ` · 已选 ${selectedCount}` : ''}`}
          </span>
        </div>
      ) : (
        /* Narrow screens: compact row with essential buttons + More overflow */
        <div className="flex items-center gap-1.5 px-3 py-2">
          <Tooltip content="上级目录">
            <button
              onClick={onGoUp}
              disabled={!canGoUp}
              className="flex shrink-0 items-center rounded-lg border border-neutral-200 px-3 py-2 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ArrowUp className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="粘贴到当前目录">
            <button onClick={onPaste} disabled={!canPaste} className="flex shrink-0 items-center rounded-lg border border-neutral-200 px-3 py-2 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40">
              <ClipboardPaste className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="转到格式工厂">
            <button
              onClick={onFormat}
              disabled={!canFormat}
              className="flex shrink-0 items-center rounded-lg border border-blue-200 px-3 py-2 text-blue-700 hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Factory className="size-4" />
            </button>
          </Tooltip>
          <Tooltip content="多媒体视图">
            <button
              onClick={onToggleMedia}
              disabled={!path}
              className={`flex shrink-0 items-center rounded-lg border px-3 py-2 disabled:cursor-not-allowed disabled:opacity-40 ${
                mediaOnly
                  ? 'border-blue-600 bg-blue-50 text-blue-700'
                  : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
              }`}
            >
              <Film className="size-4" />
            </button>
          </Tooltip>

          <span className="ml-auto shrink-0 text-xs text-neutral-400">
            {notice || `${entryCount} 项${selectedCount > 0 ? ` · ${selectedCount}` : ''}`}
          </span>

          <div ref={moreRef} className="relative shrink-0">
            <Tooltip content="更多操作">
              <button
                onClick={() => setMoreOpen((v) => !v)}
                className="flex items-center rounded-lg border border-neutral-200 px-2.5 py-1.5 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
              >
                <MoreHorizontal className="size-4" />
              </button>
            </Tooltip>
            {moreOpen && (
              <div className="absolute right-0 top-full z-50 mt-1 w-48 rounded-lg border border-neutral-200 bg-white py-1 shadow-lg">
                <MoreButton icon={<Pin className="size-4" />} label={pinned ? '取消固定' : '固定此目录'} onClick={() => { onPin(); setMoreOpen(false) }} disabled={!path} active={pinned} />
                <MoreButton icon={<Video className="size-4" />} label={isSource ? '取消多媒体源标记' : '标记为多媒体源'} onClick={() => { onToggleSource(); setMoreOpen(false) }} disabled={!path} active={isSource} />
                <div className="my-1 h-px bg-neutral-100" />
                <MoreButton icon={<Scissors className="size-4" />} label="剪切" onClick={() => { onCut(); setMoreOpen(false) }} disabled={!canMutate} />
                <MoreButton icon={<ClipboardCopy className="size-4" />} label="复制" onClick={() => { onCopy(); setMoreOpen(false) }} disabled={!canMutate} />
                <MoreButton icon={<Pencil className="size-4" />} label="重命名" onClick={() => { onRename(); setMoreOpen(false) }} disabled={selectedCount !== 1} />
                <MoreButton icon={<ReplaceAll className="size-4" />} label="批量重命名" onClick={() => { onBatchRename(); setMoreOpen(false) }} disabled={selectedCount < 2} />
                <MoreButton icon={<Trash2 className="size-4" />} label="永久删除" onClick={() => { onDelete(); setMoreOpen(false) }} disabled={!canMutate} danger />
                <div className="my-1 h-px bg-neutral-100" />
                <MoreButton icon={<ListChecks className="size-4" />} label="全选" onClick={() => { onSelectAll(); setMoreOpen(false) }} disabled={entryCount === 0} />
                <MoreButton icon={<ArrowDownUp className="size-4" />} label="反选" onClick={() => { onInvertSelection(); setMoreOpen(false) }} disabled={entryCount === 0} />
                <div className="my-1 h-px bg-neutral-100" />
                <MoreButton icon={<Layers className="size-4" />} label="标记为系列" onClick={() => { onMarkSeries(); setMoreOpen(false) }} disabled={!canMarkSeries} />
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function MoreButton({
  icon,
  label,
  onClick,
  disabled = false,
  danger = false,
  active = false,
}: {
  icon: React.ReactNode
  label: string
  onClick: () => void
  disabled?: boolean
  danger?: boolean
  active?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`flex w-full items-center gap-2.5 px-3 py-2.5 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
        danger
          ? 'text-red-600 hover:bg-red-50'
          : active
            ? 'bg-blue-50 text-blue-700'
            : 'text-neutral-700 hover:bg-neutral-50'
      }`}
    >
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0 truncate">{label}</span>
    </button>
  )
}
