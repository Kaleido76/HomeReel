import { ArrowDown, ArrowUp, ChevronRight, ClipboardCopy, ClipboardPaste, Computer, Film, Pencil, Pin, Scissors, Trash2 } from 'lucide-react'
import { pathSegments } from './path'

export type SortKey = 'name' | 'size' | 'mtime'

export interface SortState {
  key: SortKey
  dir: 'asc' | 'desc'
}

export type ClipMode = 'copy' | 'cut'

export interface Clipboard {
  mode: ClipMode
  items: string[]
}

const btnCls =
  'flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40'

// Toolbar has two rows like a Windows Explorer toolbar: the top is a full-width
// breadcrumb of the current path (every segment jumps to that ancestor), the
// bottom holds the clipboard actions (enabled once rows are checked) plus the
// view/sort controls. The grid view is a placeholder for now.
export function Toolbar({
  path,
  canGoUp,
  entryCount,
  selectedCount,
  clipboard,
  sort,
  notice,
  onSortChange,
  onNavigate,
  onGoUp,
  onCut,
  onCopy,
  onPaste,
  onRename,
  onDelete,
  onPin,
  pinned,
  mediaOnly,
  onToggleMedia,
}: {
  path: string
  canGoUp: boolean
  entryCount: number
  selectedCount: number
  clipboard: Clipboard | null
  sort: SortState
  notice: string
  onSortChange: (s: SortState) => void
  onNavigate: (p: string) => void
  onGoUp: () => void
  onCut: () => void
  onCopy: () => void
  onPaste: () => void
  onRename: () => void
  onDelete: () => void
  onPin: () => void
  pinned: boolean
  mediaOnly: boolean
  onToggleMedia: () => void
}) {
  const crumbs = pathSegments(path)

  const canMutate = selectedCount > 0
  const canPaste = !!path && !!clipboard

  return (
    <div className="shrink-0 border-b border-neutral-200 bg-white">
      {/* Breadcrumb row: full-width, every ancestor is clickable */}
      <div className="flex items-center gap-0.5 overflow-x-auto border-b border-neutral-100 px-3 py-1.5">
        <button
          onClick={() => onNavigate('')}
          title="这台电脑"
          className="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
        >
          <Computer className="size-4" />
        </button>
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

      {/* Action row */}
      <div className="flex flex-wrap items-center gap-2 px-3 py-2">
        <button onClick={onGoUp} disabled={!canGoUp} title="上级目录" className={btnCls}>
          <ArrowUp className="size-4" />
        </button>
        <button
          onClick={onPin}
          disabled={!path}
          title={pinned ? '取消固定' : '固定此目录'}
          className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-40 ${
            pinned
              ? 'border-blue-600 bg-blue-50 text-blue-700'
              : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
          }`}
        >
          <Pin className="size-4" /> {pinned ? '已固定' : '固定'}
        </button>

        <span className="mx-1 h-5 w-px shrink-0 bg-neutral-200" />

        <button onClick={onCut} disabled={!canMutate} title="剪切" className={btnCls}>
          <Scissors className="size-4" /> 剪切
        </button>
        <button onClick={onCopy} disabled={!canMutate} title="复制" className={btnCls}>
          <ClipboardCopy className="size-4" /> 复制
        </button>
        <button onClick={onPaste} disabled={!canPaste} title="粘贴到当前目录" className={btnCls}>
          <ClipboardPaste className="size-4" /> 粘贴
        </button>
        <button onClick={onRename} disabled={selectedCount !== 1} title="重命名" className={btnCls}>
          <Pencil className="size-4" /> 重命名
        </button>
        <button
          onClick={onDelete}
          disabled={!canMutate}
          title="永久删除"
          className="flex items-center gap-1.5 rounded-lg border border-red-200 px-2.5 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Trash2 className="size-4" /> 删除
        </button>

        <span className="mx-1 h-5 w-px shrink-0 bg-neutral-200" />

        <select
          value={sort.key}
          onChange={(e) => onSortChange({ ...sort, key: e.target.value as SortKey })}
          title="排序方式"
          className="rounded-lg border border-neutral-200 bg-white px-2 py-1.5 text-sm text-neutral-600 outline-none focus:border-blue-600"
        >
          <option value="name">按名称</option>
          <option value="size">按大小</option>
          <option value="mtime">按修改时间</option>
        </select>
        <button
          onClick={() => onSortChange({ ...sort, dir: sort.dir === 'asc' ? 'desc' : 'asc' })}
          title={sort.dir === 'asc' ? '升序' : '降序'}
          className="rounded-lg border border-neutral-200 p-1.5 text-neutral-600 hover:bg-neutral-50"
        >
          {sort.dir === 'asc' ? <ArrowUp className="size-4" /> : <ArrowDown className="size-4" />}
        </button>

        <span className="mx-1 h-5 w-px shrink-0 bg-neutral-200" />

        <button
          onClick={onToggleMedia}
          disabled={!path}
          title="多媒体视图：仅显示视频/音乐文件"
          className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-40 ${
            mediaOnly
              ? 'border-blue-600 bg-blue-50 text-blue-700'
              : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900'
          }`}
        >
          <Film className="size-4" /> 多媒体视图
        </button>

        <span className="ml-auto shrink-0 text-xs text-neutral-400">
          {notice || `${entryCount} 项${selectedCount > 0 ? ` · 已选 ${selectedCount}` : ''}${clipboard ? ` · 剪贴板 ${clipboard.items.length} 项` : ''}`}
        </span>
      </div>
    </div>
  )
}
