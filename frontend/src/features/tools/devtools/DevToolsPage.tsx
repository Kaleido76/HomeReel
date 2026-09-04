import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Archive,
  Check,
  ChevronRight,
  Clipboard,
  Copy,
  Eraser,
  FileClock,
  Loader2,
  Power,
  Save,
  Trash2,
} from 'lucide-react'
import {
  clearDevLog,
  getDevLogLines,
  isDevLogEnabled,
  setDevLogEnabled,
  subscribeDevLog,
  type DevLogLevel,
  type DevLogLine,
} from '../../../lib/devlog'
import {
  deleteDevLog,
  fetchDevLog,
  fetchDevLogs,
  submitDevLog,
  type DevLogSummary,
} from '../../../api/devlogs'
import { useNotify } from '../../../components/NotificationProvider'

const LEVELS: DevLogLevel[] = ['log', 'info', 'warn', 'error', 'debug']

// DevToolsPage is the 开发者工具 tool: it records frontend console/log output on
// devices where a dev console is hard to open (mobile), and can archive the
// captured lines to the backend so a PC can read them later. It mirrors the
// Android-Studio logcat idea — live capture with module & level filtering, a
// clear button, and an archive browser.
export function DevToolsPage() {
  const queryClient = useQueryClient()
  const [enabled, setEnabled] = useState(isDevLogEnabled())
  const [levelFilter, setLevelFilter] = useState<Set<DevLogLevel>>(new Set(LEVELS))
  const [moduleFilter, setModuleFilter] = useState<string>('')
  const [lines, setLines] = useState<DevLogLine[]>(getDevLogLines())
  const [viewing, setViewing] = useState<DevLogSummary | null>(null)
  const [note, setNote] = useState('')
  const { notify } = useNotify()

  useEffect(() => subscribeDevLog(() => setLines(getDevLogLines())), [])

  const modules = useMemo(() => {
    const set = new Set<string>()
    for (const l of lines) set.add(l.module)
    return [...set].sort()
  }, [lines])

  const filtered = useMemo(
    () =>
      lines.filter(
        (l) =>
          levelFilter.has(l.level) &&
          (moduleFilter === '' || l.module === moduleFilter),
      ),
    [lines, levelFilter, moduleFilter],
  )

  function toggle() {
    setDevLogEnabled(!enabled)
    setEnabled(!enabled)
  }

  function toggleLevel(l: DevLogLevel) {
    setLevelFilter((prev) => {
      const next = new Set(prev)
      if (next.has(l)) next.delete(l)
      else next.add(l)
      return next
    })
  }

  const submit = useMutation({
    mutationFn: () => submitDevLog(deviceSource(), note, getDevLogLines()),
    onSuccess: (res) => {
      setNote('')
      notify(`已归档，ID ${res.id}`)
      void queryClient.invalidateQueries({ queryKey: ['devlogs'] })
    },
    onError: (err) => notify(err instanceof Error ? err.message : '归档失败', 'error'),
  })

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900">开发者工具</h1>
        <p className="mt-1 max-w-3xl text-sm text-neutral-500">
          在无法打开浏览器开发者工具的设备（如手机）上记录前端日志：打开开关后开始采集控制台输出与带模块标记的日志，
          可实时查看 / 按模块与级别过滤 / 清除；也可把当前采集的日志归档到后端，供 PC 端选择查看或复制。
        </p>
      </div>

      {/* Capture toggle */}
      <section className="rounded-lg border border-neutral-200 bg-white p-4">
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-3">
            <Power className={`size-5 shrink-0 ${enabled ? 'text-emerald-600' : 'text-neutral-300'}`} />
            <div className="min-w-0">
              <p className="text-sm font-medium text-neutral-900">日志采集</p>
              <p className="mt-0.5 text-xs text-neutral-500">
                仅开启时记录前端日志，采集保留最近 {lines.length} / 2000 条，超出自动丢弃最旧。
              </p>
            </div>
          </div>
          <button
            onClick={toggle}
            role="switch"
            aria-checked={enabled}
            className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${
              enabled ? 'bg-emerald-500' : 'bg-neutral-300'
            }`}
          >
            <span
              className={`absolute top-0.5 size-5 rounded-full bg-white shadow transition-all ${
                enabled ? 'left-[22px]' : 'left-0.5'
              }`}
            />
          </button>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-neutral-100 pt-3">
          <span className="mr-1 text-xs font-medium text-neutral-500">级别过滤</span>
          {LEVELS.map((l) => (
            <button
              key={l}
              onClick={() => toggleLevel(l)}
              aria-pressed={levelFilter.has(l)}
              className={`flex items-center gap-1 rounded-full px-2.5 py-1 text-xs transition-colors ${
                levelFilter.has(l) ? levelColor(l) : 'bg-neutral-100 text-neutral-400'
              }`}
            >
              <span
                className={`size-1.5 rounded-full ${
                  levelFilter.has(l) ? 'bg-current' : 'bg-neutral-300'
                }`}
              />
              {l.toUpperCase()}
            </button>
          ))}
          <div className="mx-2 h-4 w-px bg-neutral-200" />
          <span className="mr-1 text-xs font-medium text-neutral-500">模块</span>
          <select
            value={moduleFilter}
            onChange={(e) => setModuleFilter(e.target.value)}
            className="rounded-md border border-neutral-300 bg-white px-2 py-1 text-xs text-neutral-700 outline-none focus:border-blue-600"
          >
            <option value="">全部模块</option>
            {modules.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <button
            onClick={() => {
              if (window.confirm('确定清空当前采集的全部日志？')) clearDevLog()
            }}
            disabled={lines.length === 0}
            className="ml-auto flex items-center gap-1 rounded border border-neutral-300 px-2.5 py-1 text-xs text-neutral-700 hover:bg-neutral-100 disabled:opacity-40"
          >
            <Eraser className="size-3.5" /> 清空（{lines.length}）
          </button>
        </div>
      </section>

      {/* Live viewer */}
      <section className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
        <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2.5">
          <p className="text-sm font-medium text-neutral-900">
            实时日志
            <span className="ml-2 text-xs font-normal text-neutral-400">
              {filtered.length} / {lines.length} 条
            </span>
          </p>
          <span className="flex items-center gap-1 text-xs text-neutral-400">
            <span
              className={`size-2 rounded-full ${enabled ? 'bg-emerald-500' : 'bg-neutral-300'}`}
            />
            {enabled ? '采集中' : '已停用'}
          </span>
        </div>
        {filtered.length === 0 ? (
          <p className="px-3 py-10 text-center text-sm text-neutral-400">
            {enabled ? '暂无日志。打开开关后，前端的 console 输出与带模块标记的日志会显示在这里。' : '采集已停用，先打开上方的开关。'}
          </p>
        ) : (
          <div className="max-h-[50vh] overflow-auto">
            {filtered.map((l, i) => (
              <div
                key={i}
                className="flex items-start gap-2 border-b border-neutral-50 px-3 py-1.5 text-xs hover:bg-neutral-50"
              >
                <span className="w-[72px] shrink-0 font-mono text-neutral-400">
                  {l.timestamp.slice(11, 23)}
                </span>
                <span
                  className={`w-12 shrink-0 rounded px-1 text-center font-medium ${levelColor(l.level)}`}
                >
                  {l.level.toUpperCase()}
                </span>
                <span className="w-24 shrink-0 truncate font-mono text-neutral-500" title={l.module}>
                  {l.module}
                </span>
                <span className="min-w-0 break-all font-mono text-neutral-800">{l.message}</span>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Archive submission */}
      <section className="rounded-lg border border-neutral-200 bg-white p-4">
        <div className="flex items-center gap-2">
          <Archive className="size-4 text-neutral-400" />
          <h2 className="text-sm font-medium text-neutral-900">归档到后端</h2>
        </div>
        <p className="mt-1 text-xs text-neutral-500">
          把当前采集的全部日志打包提交到后端保存（获得一个 ID），之后可在 PC 端选择该归档查看或复制。
          便于移动端直接把一段现场日志交给你排错。
        </p>
        <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
          <input
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="备注（可选，如：手机播放某视频卡顿）"
            className="min-w-0 flex-1 rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-800 outline-none focus:border-blue-600"
          />
          <button
            onClick={() => submit.mutate()}
            disabled={lines.length === 0 || submit.isPending}
            className="flex shrink-0 items-center justify-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
          >
            {submit.isPending ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
            提交归档（{lines.length} 条）
          </button>
        </div>
      </section>

      {/* Archive browser */}
      <section className="rounded-lg border border-neutral-200 bg-white p-4">
        <div className="flex items-center gap-2">
          <FileClock className="size-4 text-neutral-400" />
          <h2 className="text-sm font-medium text-neutral-900">归档记录</h2>
          <span className="ml-auto text-xs text-neutral-400">点击记录查看内容并复制</span>
        </div>
        <ArchiveList
          viewing={viewing}
          onView={setViewing}
          onDeleted={() => {
            setViewing(null)
            void queryClient.invalidateQueries({ queryKey: ['devlogs'] })
          }}
        />
      </section>
    </div>
  )
}

function ArchiveList({
  viewing,
  onView,
  onDeleted,
}: {
  viewing: DevLogSummary | null
  onView: (s: DevLogSummary | null) => void
  onDeleted: () => void
}) {
  const list = useQuery({ queryKey: ['devlogs'], queryFn: fetchDevLogs })
  const detail = useQuery({
    queryKey: ['devlog', viewing?.id],
    queryFn: () => fetchDevLog(viewing!.id),
    enabled: !!viewing,
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteDevLog(id),
    onSuccess: onDeleted,
  })

  const [copied, setCopied] = useState(false)

  const items = list.data?.items ?? []

  if (list.isLoading) {
    return <p className="mt-3 text-sm text-neutral-400">加载中…</p>
  }

  if (items.length === 0) {
    return <p className="mt-3 text-sm text-neutral-400">尚无归档记录。在手机上采集日志后点击上方「提交归档」。</p>
  }

  const entries = detail.data?.entries ?? []

  async function copyEntries() {
    const text = entries
      .map((e) => `${e.timestamp} [${e.level}] [${e.module}] ${e.message}`)
      .join('\n')
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="mt-3 grid gap-4 lg:grid-cols-[minmax(0,300px)_minmax(0,1fr)]">
      {/* List of archives */}
      <ul className="max-h-[50vh] space-y-1 overflow-auto lg:border-r lg:border-neutral-100 lg:pr-2">
        {items.map((s) => (
          <li key={s.id}>
            <button
              onClick={() => onView(viewing?.id === s.id ? null : s)}
              className={`flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition-colors ${
                viewing?.id === s.id ? 'bg-blue-50 text-blue-700' : 'hover:bg-neutral-50'
              }`}
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate font-mono text-xs text-neutral-400">{s.id}</span>
                <span className="block truncate text-neutral-800">{s.note || '（无备注）'}</span>
                <span className="block truncate text-xs text-neutral-400">
                  {s.source} · {s.count} 条 · {formatTime(s.created_at)}
                </span>
              </span>
              <ChevronRight className="size-4 shrink-0 text-neutral-300" />
            </button>
          </li>
        ))}
      </ul>

      {/* Detail of selected archive */}
      <div>
        {!viewing ? (
          <p className="flex h-full items-center justify-center text-sm text-neutral-300">
            选择左侧一条归档记录查看日志
          </p>
        ) : detail.isLoading ? (
          <p className="text-sm text-neutral-400">加载中…</p>
        ) : (
          <div className="flex h-full min-h-[240px] flex-col">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <span className="rounded bg-neutral-100 px-2 py-0.5 font-mono text-xs text-neutral-500">
                {viewing.id}
              </span>
              <span className="text-xs text-neutral-400">
                {viewing.note || '无备注'} · {entries.length} 条
              </span>
              <div className="ml-auto flex items-center gap-2">
                <button
                  onClick={() => void copyEntries()}
                  disabled={entries.length === 0}
                  className="flex items-center gap-1 rounded border border-neutral-300 px-2.5 py-1 text-xs text-neutral-700 hover:bg-neutral-100 disabled:opacity-40"
                >
                  {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
                  {copied ? '已复制' : '复制全部'}
                </button>
                <a
                  href={`/api/devlogs/${viewing.id}/raw`}
                  target="_blank"
                  rel="noreferrer"
                  className="flex items-center gap-1 rounded border border-neutral-300 px-2.5 py-1 text-xs text-neutral-700 hover:bg-neutral-100"
                >
                  <Clipboard className="size-3.5" /> 原始文本
                </a>
                <button
                  onClick={() => {
                    if (window.confirm(`确定删除归档 ${viewing.id}？`)) remove.mutate(viewing.id)
                  }}
                  disabled={remove.isPending}
                  className="flex items-center gap-1 rounded border border-red-200 px-2.5 py-1 text-xs text-red-600 hover:bg-red-50 disabled:opacity-40"
                >
                  <Trash2 className="size-3.5" /> 删除
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-auto rounded-md border border-neutral-200 bg-neutral-50 p-2">
              {entries.length === 0 ? (
                <p className="p-4 text-center text-sm text-neutral-400">该归档没有日志条目。</p>
              ) : (
                entries.map((e, i) => (
                  <div key={i} className="flex items-start gap-2 border-b border-neutral-100 px-1 py-1 text-xs">
                    <span className="w-[72px] shrink-0 font-mono text-neutral-400">
                      {e.timestamp.slice(11, 23)}
                    </span>
                    <span className={`w-12 shrink-0 rounded px-1 text-center font-medium ${levelColor(e.level)}`}>
                      {e.level.toUpperCase()}
                    </span>
                    <span className="w-24 shrink-0 truncate font-mono text-neutral-500" title={e.module}>
                      {e.module}
                    </span>
                    <span className="min-w-0 break-all font-mono text-neutral-800">{e.message}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function levelColor(level: DevLogLevel): string {
  const map: Record<DevLogLevel, string> = {
    log: 'bg-neutral-100 text-neutral-600',
    info: 'bg-sky-100 text-sky-700',
    debug: 'bg-neutral-100 text-neutral-500',
    warn: 'bg-amber-100 text-amber-700',
    error: 'bg-red-100 text-red-700',
  }
  return map[level]
}

function deviceSource(): string {
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
  const mobile = /Mobi|Android|iPhone/i.test(ua)
  const os = /iPhone|iPad/.test(ua)
    ? 'iOS'
    : /Android/.test(ua)
      ? 'Android'
      : /Windows/.test(ua)
        ? 'Windows'
        : /Mac/.test(ua)
          ? 'macOS'
          : /Linux/.test(ua)
            ? 'Linux'
            : '未知'
  return `${os}${mobile ? '（移动端）' : ''} · ${ua.split(')')[0] ?? ua}`
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}
