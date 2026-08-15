import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, CaseSensitive, Regex, ReplaceAll } from 'lucide-react'
import { Modal } from '../../components/Modal'
import { updateVideo } from '../../api/videos'
import type { SeriesMember } from '../../api/series'

const escRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

interface RenamePair {
  member: SeriesMember
  next: string
}

// SeriesRenameModal is the batch-rename modal for a series' member display names
// (系列详情「批量修改显示名称」): a VS Code style find/replace bar (literal or
// regex, case-insensitive) above a two-column preview of original vs. new names.
// Applying PATCHes each changed member's title (title_source becomes 'manual' and
// survives scans, ADR-015/017). Closing cancels everything.
export function SeriesRenameModal({
  seriesId,
  members,
  onClose,
}: {
  seriesId: string
  members: SeriesMember[]
  onClose: () => void
}) {
  const queryClient = useQueryClient()
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

  const pairs: RenamePair[] = useMemo(
    () => members.map((member) => ({ member, next: apply(member.episode_title || member.title) })),
    [members, apply],
  )
  const changes = pairs.filter((p) => p.next !== (p.member.episode_title || p.member.title))
  const canSubmit = !busy && valid && changes.length > 0

  const rename = useMutation({
    mutationFn: async () => {
      for (const c of changes) {
        await updateVideo(c.member.video_id, { title: c.next })
      }
    },
    onSuccess: () => {
      onClose()
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
    },
  })

  async function submit() {
    if (!canSubmit) return
    setBusy(true)
    try {
      await rename.mutateAsync()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      onClose={onClose}
      size="lg"
      title={`批量修改显示名称 · ${members.length} 项`}
      titleIcon={<ReplaceAll className="size-4 text-neutral-500" />}
      closeLabel="撤销"
      closeTitle="关闭并撤销（不执行修改）"
    >
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
          title="将查找内容视为正则表达式"
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
        <button
          onClick={() => void submit()}
          disabled={!canSubmit}
          title={!valid ? '正则表达式无效' : changes.length === 0 ? '没有匹配项' : `修改 ${changes.length} 项显示名`}
          className="flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ReplaceAll className="size-3.5" /> 开始替换
        </button>
      </div>
      {!valid && <p className="shrink-0 px-3 pb-1 text-[11px] text-red-500">正则表达式无效</p>}

      <div className="grid shrink-0 grid-cols-2 border-b border-neutral-100 bg-neutral-50 px-3 text-[11px] font-medium text-neutral-400">
        <div className="py-1.5">原显示名</div>
        <div className="flex items-center gap-1 py-1.5">
          <ArrowRight className="size-3" /> 新显示名
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {pairs.map(({ member, next }) => {
          const current = member.episode_title || member.title
          const changed = next !== current
          return (
            <div key={member.video_id} className="grid grid-cols-2 border-b border-neutral-50">
              <div className="flex min-w-0 items-center gap-2 px-3 py-1.5">
                <span className="shrink-0 rounded bg-neutral-100 px-1.5 text-[11px] font-medium text-neutral-500">
                  {member.episode_number}
                </span>
                <span className="truncate text-xs text-neutral-700" title={current}>
                  {current}
                </span>
              </div>
              <div className={`flex min-w-0 items-center gap-2 px-3 py-1.5 ${changed ? 'bg-emerald-50/60' : ''}`}>
                <span className="shrink-0 rounded bg-neutral-100 px-1.5 text-[11px] font-medium text-neutral-500">
                  {member.episode_number}
                </span>
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
    </Modal>
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
    <button
      onClick={onClick}
      type="button"
      title={title}
      className={`flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors ${
        active
          ? 'border-blue-600 bg-blue-50 text-blue-700'
          : 'border-neutral-200 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-700'
      }`}
    >
      {icon}
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}
