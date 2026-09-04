import { useState, type ReactNode } from 'react'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { Modal } from '../../../components/Modal'

// ConfirmDialog is the cache manager's destructive-action confirmation (it
// replaces the native window.confirm on this page): a small modal that states
// exactly what will be cleared, listing the affected items structurally when
// the caller supplies them (rather than one wall of prose), then runs the
// action with a busy state, surfacing failures in place.
export function ConfirmDialog({
  title,
  description,
  items,
  confirmLabel = '确认清理',
  onConfirm,
  onClose,
}: {
  title: ReactNode
  description: ReactNode
  items?: { label: ReactNode; detail?: ReactNode }[]
  confirmLabel?: string
  onConfirm: () => Promise<void>
  onClose: () => void
}) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  async function handle() {
    setPending(true)
    setError('')
    try {
      await onConfirm()
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
      setPending(false)
    }
  }

  return (
    <Modal
      onClose={onClose}
      title={title}
      titleIcon={<AlertTriangle className="size-4 shrink-0 text-red-500" />}
      closeLabel=""
      closeTitle="关闭"
      size="md"
    >
      <div className="flex flex-col p-4">
        <div className="text-sm leading-6 text-neutral-600">{description}</div>
        {items && items.length > 0 && (
          <ul className="mt-3 max-h-56 shrink overflow-y-auto rounded-md border border-neutral-200 bg-neutral-50 px-3">
            {items.map((it, i) => (
              <li
                key={i}
                className="flex items-center justify-between gap-3 border-b border-neutral-100 py-1.5 text-xs last:border-b-0"
              >
                <span className="min-w-0 truncate text-neutral-700">{it.label}</span>
                {it.detail && <span className="shrink-0 pl-2 text-neutral-400">{it.detail}</span>}
              </li>
            ))}
          </ul>
        )}
        {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={onClose}
            disabled={pending}
            className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm text-neutral-700 hover:bg-neutral-100 disabled:opacity-50"
          >
            取消
          </button>
          <button
            onClick={() => void handle()}
            disabled={pending}
            className="flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm text-white hover:bg-red-700 disabled:opacity-50"
          >
            {pending && <Loader2 className="size-3.5 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </Modal>
  )
}