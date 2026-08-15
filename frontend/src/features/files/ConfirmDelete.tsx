import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Modal } from '../../components/Modal'

// ConfirmDelete is the strong confirmation for permanent deletion: the user
// must type the exact phrase 「永久删除」 before the delete button enables.
export function ConfirmDelete({
  targets,
  onConfirm,
  onCancel,
}: {
  targets: string[]
  onConfirm: () => void
  onCancel: () => void
}) {
  const [text, setText] = useState('')
  const [pending, setPending] = useState(false)
  const ok = text.trim() === '永久删除'

  async function submit() {
    setPending(true)
    try {
      await Promise.resolve(onConfirm())
    } finally {
      setPending(false)
    }
  }

  return (
    <Modal onClose={onCancel} size="sm">
      <div className="p-4">
        <p className="text-sm font-medium text-neutral-900">永久删除 {targets.length} 项？</p>
        <p className="mt-1 text-xs text-neutral-500">此操作不可恢复，文件将直接从磁盘删除。</p>
        <div className="mt-3 max-h-32 space-y-0.5 overflow-y-auto rounded-lg bg-neutral-50 p-2">
          {targets.map((t) => (
            <p key={t} className="truncate font-mono text-xs text-neutral-600">
              {t}
            </p>
          ))}
        </div>
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="请输入「永久删除」以确认"
          autoFocus
          className="mt-3 w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-red-500"
        />
        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={onCancel}
            disabled={pending}
            className="rounded-lg border border-neutral-200 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-50"
          >
            取消
          </button>
          <button
            onClick={submit}
            disabled={!ok || pending}
            className="flex items-center gap-1.5 rounded-lg bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending && <Loader2 className="size-4 animate-spin" />}
            永久删除
          </button>
        </div>
      </div>
    </Modal>
  )
}
