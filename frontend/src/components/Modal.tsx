import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Tooltip } from './Tooltip'

// Modal is the system-wide overlay shell for a style-consistent dialog: a dimmed
// full-screen backdrop, a centered white rounded panel, an optional title bar
// (icon + title + close button) and the caller's content. Only the content
// differs between usages — every feature opens its modal the same way. The panel
// has no padding of its own so content owners keep full layout control; width is
// picked with `size`. Escape/backdrop-click close are intentionally left to the
// caller so each modal owns its close semantics.
export function Modal({
  onClose,
  title,
  titleIcon,
  closeLabel = '关闭',
  closeTitle,
  size = 'md',
  children,
}: {
  onClose: () => void
  title?: ReactNode
  titleIcon?: ReactNode
  closeLabel?: string
  closeTitle?: string
  size?: 'sm' | 'md' | 'lg'
  children: ReactNode
}) {
  const sizeClass = size === 'sm' ? 'max-w-sm' : size === 'lg' ? 'max-w-2xl' : 'max-w-lg'
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className={`flex max-h-[80vh] w-full flex-col rounded-xl border border-neutral-200 bg-white shadow-xl ${sizeClass}`}>
        {title !== undefined && (
          <div className="flex shrink-0 items-center gap-2 border-b border-neutral-100 bg-neutral-50 px-3 py-2">
            {titleIcon}
            <span className="text-xs font-medium text-neutral-700">{title}</span>
            <span className="ml-auto" />
            <Tooltip content={closeTitle ?? closeLabel}>
              <button
                onClick={onClose}
                className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-neutral-500 hover:bg-neutral-100 hover:text-red-600"
              >
                <X className="size-3.5" /> {closeLabel}
              </button>
            </Tooltip>
          </div>
        )}
        {children}
      </div>
    </div>
  )
}
