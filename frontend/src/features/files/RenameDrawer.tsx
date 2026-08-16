import { useMemo, useState } from 'react'
import { ArrowRight, CaseSensitive, Folder, Regex, ReplaceAll, X } from 'lucide-react'
import { fileStyle } from './fileType'
import { Tooltip } from '../../components/Tooltip'
import type { ClipboardItem } from './Toolbar'

const escRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

interface RenamePair {
  item: ClipboardItem
  next: string
}

// RenameDrawer is the tool-drawer content for batch rename: a VS Code style
// find/replace bar (literal or regex, case-insensitive) above a two-column
// preview of original vs. new names. Closing the drawer cancels everything —
// nothing is applied until 「开始替换」 is pressed. Requires ≥2 selections
// (enforced by the toolbar entry point).
export function RenameDrawer({
  items,
  onClose,
  onApply,
}: {
  items: ClipboardItem[]
  onClose: () => void
  onApply: (renames: { path: string; newName: string }[]) => Promise<void>
}) {
  const [find, setFind] = useState('')
  const [replace, setReplace] = useState('')
  const [ignoreCase, setIgnoreCase] = useState(false)
  const [regex, setRegex] = useState(false)
  const [busy, setBusy] = useState(false)

  const { valid, apply } = useMemo(() => {
    if (!find) return { valid: true, apply: (n: string) => n }
    const flags = ignoreCase ? 'gi' : 'g'
    try {
      const re = new RegExp(regex ? find : escRe(find), flags)
      return { valid: true, apply: (n: string) => n.replace(re, replace) }
    } catch {
      return { valid: false, apply: (n: string) => n }
    }
  }, [find, replace, ignoreCase, regex])

  const pairs: RenamePair[] = useMemo(() => items.map((item) => ({ item, next: apply(item.name) })), [items, apply])
  const changes = pairs.filter((p) => p.next !== p.item.name)
  const canSubmit = !busy && valid && changes.length > 0

  async function submit() {
    if (!canSubmit) return
    setBusy(true)
    try {
      await onApply(changes.map((c) => ({ path: c.item.path, newName: c.next })))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-100 bg-neutral-50 px-3 py-2">
        <ReplaceAll className="size-4 text-neutral-500" />
        <span className="text-xs font-medium text-neutral-700">批量重命名 · {items.length} 项</span>
        <span className="ml-auto" />
        <Tooltip content="关闭并撤销">
          <button
            onClick={onClose}
            className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-neutral-500 hover:bg-neutral-100 hover:text-red-600"
          >
            <X className="size-3.5" /> 撤销
          </button>
        </Tooltip>
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-100 px-3 py-2">
        <input
          value={find}
          onChange={(ev) => setFind(ev.target.value)}
          placeholder="查找"
          spellCheck={false}
          className={`w-44 rounded-md border px-2 py-1 text-xs outline-none focus:border-blue-600 ${
            valid ? 'border-neutral-300' : 'border-red-400'
          }`}
        />
        <ToggleBtn
          active={ignoreCase}
          onClick={() => setIgnoreCase((v) => !v)}
          title="无视大小写"
          icon={<CaseSensitive className="size-3.5" />}
          label="无视大小写"
        />
        <ToggleBtn
          active={regex}
          onClick={() => setRegex((v) => !v)}
          title="按正则表达式查找"
          icon={<Regex className="size-3.5" />}
          label="正则"
        />
        <input
          value={replace}
          onChange={(ev) => setReplace(ev.target.value)}
          placeholder="替换为"
          spellCheck={false}
          className="w-44 rounded-md border border-neutral-300 px-2 py-1 text-xs outline-none focus:border-blue-600"
        />
        <Tooltip content={!valid ? '正则表达式无效' : changes.length === 0 ? '没有匹配项' : `重命名 ${changes.length} 项`}>
          <button
            onClick={() => void submit()}
            disabled={!canSubmit}
            className="flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <ReplaceAll className="size-3.5" /> 开始替换
          </button>
        </Tooltip>
      </div>
      {!valid && <p className="shrink-0 px-3 pb-1 text-[11px] text-red-500">正则表达式无效</p>}

      <div className="grid shrink-0 grid-cols-2 border-b border-neutral-100 bg-neutral-50 px-3 text-[11px] font-medium text-neutral-400">
        <div className="py-1.5">原文件</div>
        <div className="flex items-center gap-1 py-1.5">
          <ArrowRight className="size-3" /> 新文件名
        </div>
      </div>

      <div className="max-h-64 overflow-y-auto">
        {pairs.map(({ item, next }) => {
          const changed = next !== item.name
          return (
            <div key={item.path} className="grid grid-cols-2 border-b border-neutral-50">
              <div className="flex min-w-0 items-center gap-2 px-3 py-1.5">
                <CellIcon item={item} />
                <span className="truncate text-xs text-neutral-700" title={item.name}>
                  {item.name}
                </span>
              </div>
              <div className={`flex min-w-0 items-center gap-2 px-3 py-1.5 ${changed ? 'bg-emerald-50/60' : ''}`}>
                <CellIcon item={item} />
                <span
                  className={`truncate text-xs ${changed ? 'font-medium text-emerald-700' : 'text-neutral-500'}`}
                  title={next}
                >
                  {next}
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function ToggleBtn({
  active,
  onClick,
  title,
  icon,
  label,
}: {
  active: boolean
  onClick: () => void
  title: string
  icon: React.ReactNode
  label: string
}) {
  return (
    <Tooltip content={title}>
      <button
        onClick={onClick}
        type="button"
        className={`flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors ${
          active
            ? 'border-blue-600 bg-blue-50 text-blue-700'
            : 'border-neutral-200 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-700'
        }`}
      >
        {icon}
        <span className="hidden sm:inline">{label}</span>
      </button>
    </Tooltip>
  )
}

function CellIcon({ item }: { item: ClipboardItem }) {
  const style = fileStyle(item.name)
  return item.is_dir ? (
    <Folder className="size-3.5 shrink-0 text-neutral-400" />
  ) : (
    <style.icon className={`size-3.5 shrink-0 ${style.className}`} />
  )
}
