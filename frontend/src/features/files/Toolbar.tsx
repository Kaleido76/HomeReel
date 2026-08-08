import { ArrowUp, ChevronRight, ClipboardCopy, ClipboardPaste, Computer, Film, Pencil, Pin, Scissors, Trash2, Video } from 'lucide-react'
import { pathSegments } from './path'
import { Tooltip } from './Tooltip'

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
// view/sort controls. The grid view is a placeholder for now.
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
  onPin,
  pinned,
  isSource,
  onToggleSource,
  mediaOnly,
  onToggleMedia,
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
  onPin: () => void
  pinned: boolean
  isSource: boolean
  onToggleSource: () => void
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
        <Tooltip tip="上级目录">
          <button
            onClick={onGoUp}
            disabled={!canGoUp}
            className="flex items-center rounded-lg border border-neutral-200 px-3.5 py-2 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <ArrowUp className="size-4" />
          </button>
        </Tooltip>
        <Tooltip tip={pinned ? '取消固定' : '固定此目录'}>
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
        <Tooltip tip={isSource ? '取消多媒体源标记（已入库内容不会移除）' : '将当前目录标记为多媒体源并触发扫描'}>
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

        <Tooltip tip="剪切">
          <button onClick={onCut} disabled={!canMutate} className={btnCls}>
            <Scissors className="size-4" />
          </button>
        </Tooltip>
        <Tooltip tip="复制">
          <button onClick={onCopy} disabled={!canMutate} className={btnCls}>
            <ClipboardCopy className="size-4" />
          </button>
        </Tooltip>
        <Tooltip tip="粘贴到当前目录">
          <button onClick={onPaste} disabled={!canPaste} className={btnCls}>
            <ClipboardPaste className="size-4" />
          </button>
        </Tooltip>
        <Tooltip tip="重命名">
          <button onClick={onRename} disabled={selectedCount !== 1} className={btnCls}>
            <Pencil className="size-4" />
          </button>
        </Tooltip>
        <Tooltip tip="永久删除">
          <button
            onClick={onDelete}
            disabled={!canMutate}
            className="flex items-center rounded-lg border border-red-200 px-2.5 py-1.5 text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Trash2 className="size-4" />
          </button>
        </Tooltip>

        <Tooltip tip="多媒体视图：仅显示视频/音乐文件">
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
    </div>
  )
}
