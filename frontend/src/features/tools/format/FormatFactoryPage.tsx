import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, FileVideo, Folder, Loader2, Lock, RefreshCw, XCircle } from 'lucide-react'
import type { Job } from '../../../api/jobs'
import { isActiveJob } from '../../../api/jobs'
import { convertPaths, probePaths, type ConvertProbe } from '../../../api/convert'
import { formatEta } from '../../../lib/format'
import { useJobs, jobsKey } from '../../jobs/useJobs'
import { clearPending, intendedOutput, usePending } from './queue'
import {
  CONVERT_PRESETS,
  DEFAULT_PARAMS,
  matchPreset,
  crfChoices,
  kbpsChoices,
  type ConvertParams,
} from './presets'

// UNIVERSAL_AUDIO mirrors the backend's universalMp4Audio whitelist.
const UNIVERSAL_AUDIO = new Set(['aac', 'mp3'])

// FormatFactoryPage is the 格式工厂 tool: it turns any local video (or a whole
// folder/series of videos) into a browser-playable faststart MP4 copy. The
// operations panel probes the selected files (codecs / bitmap subtitles) to
// guide the user and disable irrelevant options, offers the three presets as
// quick-fill buttons over a always-present parameter form, and is masked until
// files are selected. Below it the pending queue shows the chosen files (each
// with its probe badges) and the queue shows every conversion — active ones
// with live progress, finished ones as history.
export function FormatFactoryPage() {
  const queryClient = useQueryClient()
  const pending = usePending()
  const jobsQuery = useJobs()
  const [params, setParams] = useState<ConvertParams>({ ...DEFAULT_PARAMS })

  const convertJobs = useMemo(
    () => (jobsQuery.data?.jobs ?? []).filter((j) => j.type === 'convert'),
    [jobsQuery.data],
  )
  // Active jobs first (oldest running on top), then finished history, newest
  // first — so a long-running conversion never gets buried under newer rows.
  const activeJobs = useMemo(
    () => convertJobs.filter(isActiveJob).sort((a, b) => a.created_at.localeCompare(b.created_at)),
    [convertJobs],
  )
  const historyJobs = useMemo(() => convertJobs.filter((j) => !isActiveJob(j)), [convertJobs])

  const patchParams = (patch: Partial<ConvertParams>) => {
    setParams((prev) => ({ ...prev, ...patch }))
  }

  const pendingPaths = useMemo(() => pending.map((p) => p.path), [pending])
  const probeQuery = useQuery({
    queryKey: ['convert-probe', pendingPaths],
    queryFn: () => probePaths(pendingPaths),
    enabled: pendingPaths.length > 0,
    staleTime: 30_000,
  })
  // Normalize codec lists to arrays — a legacy/edge response must never deliver
  // null where the badges and facts read .length/.flatMap.
  const probes = useMemo(
    () =>
      (probeQuery.data?.results ?? []).map((p) => ({
        ...p,
        audio_codecs: p.audio_codecs ?? [],
        subtitle_codecs: p.subtitle_codecs ?? [],
      })),
    [probeQuery.data],
  )

  const facts = useMemo(() => {
    const anySubtitle = probes.some((p) => p.subtitle_codecs.length > 0)
    const bitmapCount = probes.filter((p) => p.has_bitmap_subtitle).length
    const nonUniversal = [...new Set(probes.flatMap((p) => p.audio_codecs).filter((c) => !UNIVERSAL_AUDIO.has(c)))]
    return { anySubtitle, bitmapCount, nonUniversal }
  }, [probes])
  // Disabling rules only engage once a probe result is in, so a still-loading
  // probe never wrongly locks an option.
  const hasNonUniversal = probes.length > 0 && facts.nonUniversal.length > 0
  const noSubtitle = probes.length > 0 && !facts.anySubtitle
  // "保留原音" is impossible to honor with non-universal audio; clamp it to
  // smart so the form never submits a state that produces a silent-on-Windows
  // file.
  const effectiveParams: ConvertParams =
    hasNonUniversal && params.audio === 'copy' ? { ...params, audio: 'smart' } : params

  const convert = useMutation({
    mutationFn: (args: { paths: string[]; params: ConvertParams }) => convertPaths(args.paths, args.params),
    onSuccess: () => {
      clearPending()
      // 转换是后台任务，立即刷新任务指示器让顶部图标马上旋转。
      void queryClient.invalidateQueries({ queryKey: jobsKey })
    },
  })

  const hasPending = pending.length > 0
  const hasJobs = convertJobs.length > 0

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900">格式工厂</h1>
        <p className="mt-1 max-w-2xl text-sm text-neutral-500">
          把任意视频转换为 Faststart MP4 副本，浏览器可直接 Range 播放、可拖动进度条——无法直接播放的格式需先经此转换。
          在「文件」页签选中视频或文件夹后点工具栏的「格式工厂」按钮加入待转换队列，选择转换方式后开始转换。
        </p>
      </div>

      <OperationsPanel
        params={effectiveParams}
        onPatch={patchParams}
        enabled={hasPending}
        probing={probeQuery.isFetching}
        probeCount={probes.length}
        facts={facts}
        hasNonUniversal={hasNonUniversal}
        noSubtitle={noSubtitle}
      />

      {hasPending && (
        <section className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
          <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2.5">
            <p className="text-sm font-medium text-neutral-900">待转换（{pending.length} 项）</p>
            <button
              onClick={() => convert.mutate({ paths: pending.map((p) => p.path), params: effectiveParams })}
              disabled={convert.isPending}
              className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
            >
              {convert.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
              开始转换
            </button>
          </div>
          <ul className="divide-y divide-neutral-100">
            {pending.map((p) => (
              <li key={p.path} className="flex items-center gap-3 px-3 py-2.5">
                {p.is_dir ? (
                  <Folder className="size-4 shrink-0 text-amber-500" />
                ) : (
                  <FileVideo className="size-4 shrink-0 text-blue-500" />
                )}
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-1 truncate text-sm font-medium text-neutral-800" title={p.path}>
                    <span className="min-w-0 truncate">{p.name}</span>
                    {p.is_dir ? (
                      <DirProbeBadges dirPath={p.path} probes={probes} />
                    ) : (
                      <FileProbeBadges probe={probes.find((x) => x.path === p.path)} />
                    )}
                  </p>
                  <p className="truncate text-xs text-neutral-400" title={intendedOutput(p)}>
                    → {intendedOutput(p)}
                  </p>
                </div>
                <span className="shrink-0 rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500">
                  {p.is_dir ? '系列/文件夹' : '单集'}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {!hasPending && !hasJobs && (
        <div className="rounded-lg border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          暂无转换任务。在「文件」页签选中视频或文件夹，点工具栏「格式工厂」按钮后这里会出现待转换队列与转换历史。
        </div>
      )}

      {hasJobs && (
        <section className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
          <div className="border-b border-neutral-100 px-3 py-2.5">
            <p className="text-sm font-medium text-neutral-900">
              转换队列<span className="ml-2 text-xs font-normal text-neutral-400">{convertJobs.length} 个任务</span>
            </p>
          </div>
          {activeJobs.length > 0 && (
            <>
              <p className="sticky top-0 border-b border-neutral-100 bg-neutral-50 px-3 py-1.5 text-xs font-medium text-neutral-400">
                进行中
              </p>
              <ul className="divide-y divide-neutral-100">
                {activeJobs.map((j) => (
                  <ConvertJobRow key={j.id} job={j} />
                ))}
              </ul>
            </>
          )}
          {historyJobs.length > 0 && (
            <>
              <p className="sticky top-0 border-b border-neutral-100 bg-neutral-50 px-3 py-1.5 text-xs font-medium text-neutral-400">
                历史
              </p>
              <ul className="divide-y divide-neutral-100">
                {historyJobs.map((j) => (
                  <ConvertJobRow key={j.id} job={j} />
                ))}
              </ul>
            </>
          )}
        </section>
      )}

      {convert.isError && (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
          {convert.error instanceof Error ? convert.error.message : '转换提交失败'}
        </p>
      )}
      {convert.isSuccess && (convert.data.errors?.length ?? 0) > 0 && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
          {convert.data.errors?.map((e) => (
            <p key={e.path} className="truncate" title={e.path}>
              {e.path}：{e.message}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

// OperationsPanel is the 操作面板: the preset buttons merely quick-fill the
// always-present parameter form below; a probe strip guides the user and
// disables options that make no sense for the selected files. The whole panel
// is masked until files are selected.
function OperationsPanel({
  params,
  onPatch,
  enabled,
  probing,
  probeCount,
  facts,
  hasNonUniversal,
  noSubtitle,
}: {
  params: ConvertParams
  onPatch: (patch: Partial<ConvertParams>) => void
  enabled: boolean
  probing: boolean
  probeCount: number
  facts: { anySubtitle: boolean; bitmapCount: number; nonUniversal: string[] }
  hasNonUniversal: boolean
  noSubtitle: boolean
}) {
  const preset = matchPreset(params)
  const video = params.video
  const audio = params.audio
  // Lossless copy never re-encodes, so CRF is irrelevant; 保留原音 copies the
  // original track, so the AAC bitrate is irrelevant.
  const crfDisabled = video === 'copy'
  const kbpsDisabled = audio === 'copy'
  // Burning only exists for a re-encode preset and only when a subtitle track
  // exists (the fast MP4 preset burns bitmap subtitles automatically).
  const burnDisabled = video === 'copy' || noSubtitle
  const burnHint =
    video === 'copy'
      ? '快速 MP4 无损优先，位图字幕自动降级为烧录'
      : noSubtitle
        ? '所选文件均无字幕轨'
        : ''
  const keepOriginalDisabled = hasNonUniversal

  return (
    <div className="relative">
      <section
        aria-disabled={!enabled}
        className="overflow-hidden rounded-lg border border-neutral-200 bg-white"
      >
        <div className="border-b border-neutral-100 px-3 py-2.5">
          <p className="text-sm font-medium text-neutral-900">操作面板</p>
        </div>
        <div className="space-y-3 p-3">
          <div>
            <p className="mb-1.5 text-xs font-medium text-neutral-500">快速填充</p>
            <div className="flex flex-wrap gap-2">
              {CONVERT_PRESETS.map((p) => {
                const active = !!preset && preset.id === p.id
                const Icon = p.icon
                return (
                  <button
                    key={p.id}
                    onClick={() => onPatch(p.params)}
                    aria-pressed={active}
                    title={p.description}
                    className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors ${
                      active
                        ? 'border-blue-600 bg-blue-50 text-blue-700'
                        : 'border-neutral-200 bg-white text-neutral-700 hover:bg-neutral-50 hover:text-neutral-900'
                    }`}
                  >
                    <Icon className="size-4" />
                    {p.label}
                  </button>
                )
              })}
            </div>
          </div>

          <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3">
            <p className="mb-2 text-xs font-medium text-neutral-500">转换参数</p>
            <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
              <ParamField label="视频编码">
                <select
                  value={video}
                  onChange={(e) => onPatch({ video: e.target.value as ConvertParams['video'] })}
                  className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm text-neutral-700 outline-none focus:border-blue-600"
                >
                  <option value="copy">无损流拷贝</option>
                  <option value="h264">H.264</option>
                  <option value="h265">H.265</option>
                </select>
              </ParamField>
              <ParamField label="清晰度（CRF）">
                <select
                  value={params.vcrf}
                  disabled={crfDisabled}
                  onChange={(e) => onPatch({ vcrf: Number(e.target.value) })}
                  title={crfDisabled ? '无损流拷贝不改变画质' : undefined}
                  className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm text-neutral-700 outline-none focus:border-blue-600 disabled:opacity-50"
                >
                  {crfChoices().map((c) => (
                    <option key={c.value} value={c.value}>
                      {c.label}
                    </option>
                  ))}
                </select>
              </ParamField>
              <ParamField label="音频">
                <select
                  value={audio}
                  onChange={(e) => onPatch({ audio: e.target.value as ConvertParams['audio'] })}
                  className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm text-neutral-700 outline-none focus:border-blue-600"
                >
                  <option value="smart">智能（无损优先）</option>
                  <option value="copy" disabled={keepOriginalDisabled}>
                    保留原音
                  </option>
                  <option value="aac">AAC 重编码</option>
                </select>
              </ParamField>
              <ParamField label="音频码率（AAC）">
                <select
                  value={params.akbps}
                  disabled={kbpsDisabled}
                  onChange={(e) => onPatch({ akbps: Number(e.target.value) })}
                  title={kbpsDisabled ? '保留原音时不重编码音频' : undefined}
                  className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm text-neutral-700 outline-none focus:border-blue-600 disabled:opacity-50"
                >
                  {kbpsChoices().map((k) => (
                    <option key={k} value={k}>
                      {k} kbps
                    </option>
                  ))}
                </select>
              </ParamField>
            </div>

            <div className="mt-3 flex flex-wrap items-center gap-3 border-t border-neutral-200 pt-3">
              <label className="flex cursor-pointer items-center gap-1.5 text-sm text-neutral-700">
                <input
                  type="checkbox"
                  checked={params.burn}
                  disabled={burnDisabled}
                  onChange={(e) => onPatch({ burn: e.target.checked })}
                  className="accent-blue-600 disabled:opacity-50"
                />
                烧录首选字幕进画面
              </label>
              {burnHint && <span className="text-xs text-neutral-400">{burnHint}</span>}
              <span className="ml-auto text-xs text-neutral-400">
                {preset ? `当前预设：${preset.label}` : '已自定义参数'}
              </span>
            </div>
          </div>

          <ProbeGuidance probing={probing} probeCount={probeCount} facts={facts} keepOriginalDisabled={keepOriginalDisabled} />
        </div>
      </section>

      {!enabled && (
        <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-1.5 rounded-lg bg-white/75">
          <Lock className="size-5 text-neutral-400" />
          <p className="text-sm font-medium text-neutral-500">未选择要转换的文件</p>
          <p className="text-xs text-neutral-400">在「文件」页签选中视频或文件夹，点工具栏「格式工厂」加入待转换队列</p>
        </div>
      )}
    </div>
  )
}

// ProbeGuidance is the 检测信息 strip: aggregated facts about the selected
// files, each line explaining a rule the panel has applied.
function ProbeGuidance({
  probing,
  probeCount,
  facts,
  keepOriginalDisabled,
}: {
  probing: boolean
  probeCount: number
  facts: { anySubtitle: boolean; bitmapCount: number; nonUniversal: string[] }
  keepOriginalDisabled: boolean
}) {
  const lines: string[] = []
  if (probing && probeCount === 0) {
    lines.push('正在检测所选文件…')
  }
  if (facts.bitmapCount > 0) {
    lines.push(`${facts.bitmapCount} 个文件含位图字幕（PGS/VobSub），快速 MP4 将自动降级为烧录重编码`)
  }
  if (keepOriginalDisabled) {
    lines.push(
      `检测到 ${facts.nonUniversal.map((c) => c.toUpperCase()).join(' / ')} 音频，已禁用「保留原音」选项，将自动转为 AAC`,
    )
  }
  if (facts.anySubtitle && facts.bitmapCount === 0) {
    lines.push('检测到文本字幕，将保留为 mov_text 轨')
  }
  if (probeCount > 0 && !facts.anySubtitle) {
    lines.push('所选文件均无字幕轨，烧录字幕选项已禁用')
  }

  return (
    <div className="rounded-lg border border-amber-200/60 bg-amber-50/50 px-3 py-2 text-xs text-amber-800">
      <p className="mb-0.5 font-medium text-amber-700">
        {probeCount > 0 ? `检测 ${probeCount} 个文件` : probing ? '正在检测所选文件…' : '检测所选文件'}
      </p>
      {lines.length > 0 && (
        <ul className="list-inside list-disc space-y-0.5">
          {lines.map((l) => (
            <li key={l}>{l}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

function ParamField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs text-neutral-500">{label}</span>
      {children}
    </label>
  )
}

function Badge({ tone, children }: { tone: 'neutral' | 'amber' | 'emerald'; children: ReactNode }) {
  const cls =
    tone === 'amber'
      ? 'bg-amber-100 text-amber-700'
      : tone === 'emerald'
        ? 'bg-emerald-100 text-emerald-700'
        : 'bg-neutral-100 text-neutral-500'
  return <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] ${cls}`}>{children}</span>
}

// FileProbeBadges renders the probe facts of one selected file as small chips.
function FileProbeBadges({ probe }: { probe?: ConvertProbe }) {
  if (!probe) return null
  const nonUni = [...new Set(probe.audio_codecs.filter((c) => !UNIVERSAL_AUDIO.has(c)))]
  return (
    <span className="inline-flex shrink-0 gap-1">
      {probe.has_bitmap_subtitle && <Badge tone="amber">位图字幕</Badge>}
      {probe.subtitle_codecs.length > 0 && !probe.has_bitmap_subtitle && <Badge tone="neutral">文本字幕</Badge>}
      {nonUni.length > 0 && <Badge tone="amber">{nonUni.map((c) => c.toUpperCase()).join('/')}</Badge>}
    </span>
  )
}

// DirProbeBadges aggregates the probe facts of a folder selection's videos.
function DirProbeBadges({ dirPath, probes }: { dirPath: string; probes: ConvertProbe[] }) {
  const prefix = dirPath.endsWith('\\') || dirPath.endsWith('/') ? dirPath : dirPath + '\\'
  const children = probes.filter((p) => p.path.startsWith(prefix))
  if (children.length === 0) return null
  const bitmap = children.filter((c) => c.has_bitmap_subtitle).length
  const nonUni = [...new Set(children.flatMap((c) => c.audio_codecs).filter((c) => !UNIVERSAL_AUDIO.has(c)))]
  return (
    <span className="inline-flex shrink-0 gap-1">
      <Badge tone="neutral">{children.length} 个视频</Badge>
      {bitmap > 0 && <Badge tone="amber">位图字幕 ×{bitmap}</Badge>}
      {nonUni.length > 0 && <Badge tone="amber">{nonUni.map((c) => c.toUpperCase()).join('/')}</Badge>}
    </span>
  )
}

function ConvertJobRow({ job }: { job: Job }) {
  const active = isActiveJob(job)
  const determinate = job.progress >= 0
  const pct = Math.round(job.progress * 100)
  const eta = job.eta_seconds != null ? formatEta(job.eta_seconds) : ''
  const presetBadge = convertPresetLabel(job)

  return (
    <li className="flex items-start gap-2 px-3 py-2.5">
      {job.status === 'failed' ? (
        <XCircle className="mt-0.5 size-4 shrink-0 text-red-500" />
      ) : job.status === 'done' ? (
        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600" />
      ) : (
        <RefreshCw className={`mt-0.5 size-4 shrink-0 ${active ? 'animate-spin text-blue-600' : 'text-neutral-400'}`} />
      )}
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-2 truncate text-sm font-medium text-neutral-800" title={job.target}>
          <span className="min-w-0 truncate">{job.name}</span>
          {presetBadge && (
            <span className="shrink-0 rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">
              {presetBadge}
            </span>
          )}
        </p>
        {active && (
          <div className="mt-1 h-1 overflow-hidden rounded-sm bg-neutral-200">
            {determinate ? (
              <div className="h-full bg-blue-600 transition-all" style={{ width: `${pct}%` }} />
            ) : (
              <div className="indeterminate-bar h-full w-1/2 bg-blue-600" />
            )}
          </div>
        )}
        {active && determinate && (
          <p className="mt-1 text-xs text-neutral-400">
            {pct}%
            {eta ? ` · 预计还需 ${eta}` : ''}
          </p>
        )}
        {active && !determinate && (
          <p className="mt-1 text-xs text-neutral-400">{job.status === 'queued' ? '排队中…' : '处理中…'}</p>
        )}
        {active && job.subtask && <p className="mt-1 truncate text-xs text-neutral-500">{job.subtask}</p>}
        {job.status === 'failed' && <p className="mt-1 truncate text-xs text-red-500">{job.error}</p>}
        {job.status === 'done' && <p className="mt-1 text-xs text-neutral-400">已完成</p>}
      </div>
    </li>
  )
}

// convertPresetLabel reads the preset back out of a job's stored params so the
// queue can show which operations-panel tool each task used ("自定义" once the
// user deviated). Unknown/legacy payloads return empty (no badge).
function convertPresetLabel(job: Job): string {
  try {
    const meta = JSON.parse(job.extra) as { params?: Partial<ConvertParams> }
    const params: ConvertParams = { ...DEFAULT_PARAMS, ...(meta.params ?? {}) }
    const preset = matchPreset(params)
    return preset ? preset.label : '自定义'
  } catch {
    return ''
  }
}
