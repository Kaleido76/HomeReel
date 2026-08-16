import { Copy, Folder, Scissors, X } from 'lucide-react'
import { fileStyle } from './fileType'
import { Tooltip } from '../../components/Tooltip'
import type { ClipboardItem, ClipMode } from './Toolbar'

// ClipboardDrawer is the tool-drawer content that replaces the toolbar's
// 「剪贴板 N 项」text: after a cut/copy it lists the pending files as a compact
// container. It is independent of the list checkboxes — checkbox changes never
// touch it, and it only supports removing a single item or clearing all.
export function ClipboardDrawer({
  items,
  mode,
  onRemove,
  onClear,
}: {
  items: ClipboardItem[]
  mode: ClipMode
  onRemove: (path: string) => void
  onClear: () => void
}) {
  const ModeIcon = mode === 'cut' ? Scissors : Copy
  return (
    <div className="flex flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-100 bg-neutral-50 px-3 py-2">
        <ModeIcon className="size-3.5 text-neutral-500" />
        <span className="text-xs font-medium text-neutral-700">
          {mode === 'cut' ? '待移动' : '待复制'} · {items.length} 项
        </span>
        <span className="ml-auto" />
        <Tooltip content="清空抽屉">
          <button
            onClick={onClear}
            disabled={items.length === 0}
            className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-neutral-500 hover:bg-neutral-100 hover:text-red-600 disabled:opacity-40"
          >
            <X className="size-3.5" /> 全部清除
          </button>
        </Tooltip>
      </div>
      {/* min-h keeps a comfortable drawer for 1-3 selections (≈5 rows), while
          many selections scroll inside max-h. */}
      <div className="max-h-52 min-h-40 overflow-y-auto">
        {items.map((it) => {
          const style = fileStyle(it.name)
          return (
            <div key={it.path} className="flex items-center gap-2 border-b border-neutral-50 px-3 py-1.5 hover:bg-neutral-50">
              {it.is_dir ? (
                <Folder className="size-3.5 shrink-0 text-neutral-400" />
              ) : (
                <style.icon className={`size-3.5 shrink-0 ${style.className}`} />
              )}
              <span className="min-w-0 flex-1 truncate text-xs text-neutral-700" title={it.path}>
                {it.name}
              </span>
              <Tooltip content="从抽屉移除">
                <button
                  onClick={() => onRemove(it.path)}
                  className="shrink-0 rounded p-0.5 text-neutral-400 hover:bg-neutral-200 hover:text-red-600"
                >
                  <X className="size-3.5" />
                </button>
              </Tooltip>
            </div>
          )
        })}
      </div>
    </div>
  )
}
