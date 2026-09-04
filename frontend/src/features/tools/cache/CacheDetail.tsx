import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Eraser, Layers, Loader2, Sparkles, Trash2 } from 'lucide-react'
import {
  clearPrefs,
  clearSeriesPrefs,
  clearSeriesRemux,
  clearSeriesSubtitles,
  clearSubtitleCache,
  clearVideoRemux,
  pregenSeriesSubtitles,
  pregenSubtitles,
  type CacheRemuxGroup,
  type PlaybackPrefsCacheEntry,
  type SeriesPrefsCacheEntry,
  type SubtitleCacheGroup,
} from '../../../api/cache'
import { fetchSeriesMembers, seriesPosterUrl } from '../../../api/series'
import { coverUrl } from '../../../api/videos'
import { isActiveJob } from '../../../api/jobs'
import { useJobs } from '../../jobs/useJobs'
import { useNotify } from '../../../components/NotificationProvider'
import { formatBytes } from '../../../lib/format'
import { prefsSummary, seriesPrefsSummary, type SeriesCacheInfo, type StandaloneCacheInfo } from './model'
import { VideoCacheRow } from './VideoCacheRow'
import { ConfirmDialog } from './ConfirmDialog'

// CacheDetail renders the right-hand pane (or the whole column on narrow
// screens) for the selected management unit. Series and standalone share the
// same header/action layout (the standalone just has no member panel below).

// runClear executes a destructive clear, refreshes the cache overview and
// surfaces the outcome (or an error) through the global notification banner.
async function runClear(
  fn: () => Promise<unknown>,
  text: string,
  onChanged: () => Promise<void>,
  notify: (message: string, type?: 'success' | 'warning' | 'error') => void,
) {
  try {
    await fn()
    await onChanged()
    notify(text)
  } catch (err) {
    notify(err instanceof Error ? err.message : '操作失败', 'error')
  }
}

// DetailActions is the shared「预生成 / 清理字幕 / 清理 Remux / 清理记忆」button
// row of the series and standalone detail headers; the two panes differ only in
// the hint text and the pre-generation target.
function DetailActions({
  pregenBusy,
  activePregen,
  onPregen,
  hasSubtitles,
  onClearSubtitles,
  hasRemux,
  onClearRemux,
  hasMemory,
  onClearPrefs,
  hint,
}: {
  pregenBusy: boolean
  activePregen: boolean
  onPregen: () => void
  hasSubtitles: boolean
  onClearSubtitles: () => void
  hasRemux: boolean
  onClearRemux: () => void
  hasMemory: boolean
  onClearPrefs: () => void
  hint: string
}) {
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-neutral-100 pt-3">
      <button
        onClick={() => void onPregen()}
        disabled={pregenBusy || activePregen}
        className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {pregenBusy || activePregen ? <Loader2 className="size-4 animate-spin" /> : <Sparkles className="size-4" />}
        {activePregen ? '预生成中…' : '预生成缓存'}
      </button>
      <button
        onClick={onClearSubtitles}
        disabled={!hasSubtitles}
        className="flex items-center gap-1.5 rounded-md border border-neutral-300 px-3 py-1.5 text-sm text-neutral-700 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40"
      >
        <Eraser className="size-4" /> 清理字幕
      </button>
      <button
        onClick={onClearRemux}
        disabled={!hasRemux}
        className="flex items-center gap-1.5 rounded-md border border-neutral-300 px-3 py-1.5 text-sm text-neutral-700 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40"
      >
        <Eraser className="size-4" /> 清理 Remux
      </button>
      <button
        onClick={onClearPrefs}
        disabled={!hasMemory}
        className="flex items-center gap-1.5 rounded-md border border-neutral-300 px-3 py-1.5 text-sm text-neutral-700 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40"
      >
        <Trash2 className="size-4" /> 清理记忆
      </button>
      <span className="text-xs text-neutral-400">{hint}</span>
    </div>
  )
}

export function SeriesCacheDetail({
  info,
  subtitlesByVideo,
  remuxByVideo,
  prefsByVideo,
  seriesPrefs,
  narrow,
  onBack,
  onChanged,
}: {
  info: SeriesCacheInfo
  subtitlesByVideo: Map<string, SubtitleCacheGroup>
  remuxByVideo: Map<string, CacheRemuxGroup>
  prefsByVideo: Map<string, PlaybackPrefsCacheEntry>
  seriesPrefs?: SeriesPrefsCacheEntry
  narrow: boolean
  onBack?: () => void
  onChanged: () => Promise<void>
}) {
  const [dialog, setDialog] = useState<null | 'subtitles' | 'prefs' | 'remux'>(null)
  const [pregenBusy, setPregenBusy] = useState(false)
  const { notify } = useNotify()
  const jobs = useJobs()
  const membersQuery = useQuery({
    queryKey: ['series-members', info.series.id],
    queryFn: () => fetchSeriesMembers(info.series.id),
  })
  const members = membersQuery.data?.members ?? []
  const cachedCount = members.filter(
    (m) => subtitlesByVideo.has(m.video_id) || prefsByVideo.has(m.video_id) || remuxByVideo.has(m.video_id),
  ).length

  const activePregen = (jobs.data?.jobs ?? []).find(
    (j) => j.type === 'pregen' && j.target === info.series.id && isActiveJob(j),
  )
  const hasSubtitles = info.subtitleFiles > 0
  const hasMemory = info.hasSeriesPrefs || info.memberPrefs > 0
  const hasRemux = info.remuxFiles > 0

  const subtitleItems = members
    .filter((m) => subtitlesByVideo.has(m.video_id))
    .map((m) => {
      const g = subtitlesByVideo.get(m.video_id)!
      return { label: `E${m.episode_number} ${m.title}`, detail: `${g.files.length} 轨 · ${formatBytes(g.bytes)}` }
    })
  const remuxItems = members
    .filter((m) => remuxByVideo.has(m.video_id))
    .map((m) => {
      const r = remuxByVideo.get(m.video_id)!
      return { label: `E${m.episode_number} ${m.title}`, detail: `${r.files} 个 · ${formatBytes(r.bytes)}` }
    })
  const memoryItems = [
    ...(seriesPrefs ? [{ label: `系列共享记忆（${info.series.name}）`, detail: seriesPrefsSummary(seriesPrefs) }] : []),
    ...members
      .filter((m) => prefsByVideo.has(m.video_id))
      .map((m) => ({
        label: `E${m.episode_number} ${m.title}`,
        detail: prefsSummary(prefsByVideo.get(m.video_id)!),
      })),
  ]

  async function submitPregen() {
    setPregenBusy(true)
    try {
      await pregenSeriesSubtitles(info.series.id)
      notify('已提交预生成任务，可在顶部任务栏查看进度')
    } catch (err) {
      notify(err instanceof Error ? err.message : '任务提交失败', 'error')
    } finally {
      setPregenBusy(false)
    }
  }

  async function clearSubtitles() {
    setDialog(null)
    await runClear(() => clearSeriesSubtitles(info.series.id), `已清理「${info.series.name}」的字幕缓存`, onChanged, notify)
  }

  async function clearRemux() {
    setDialog(null)
    await runClear(() => clearSeriesRemux(info.series.id), `已清理「${info.series.name}」的 Remux 缓存`, onChanged, notify)
  }

  async function clearMemory() {
    setDialog(null)
    await runClear(async () => {
      await clearSeriesPrefs(info.series.id)
      const targets = [...prefsByVideo.values()].filter((p) => p.show_id === info.series.show_id)
      await Promise.all(targets.map((p) => clearPrefs(p.video_id)))
    }, `已清理「${info.series.name}」的播放记忆`, onChanged, notify)
  }

  return (
    <div className="flex flex-col gap-3">
      {narrow && onBack && (
        <button
          onClick={onBack}
          className="flex shrink-0 items-center gap-1 self-start rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-sm text-neutral-600 hover:bg-neutral-100"
        >
          <ArrowLeft className="size-4" /> 返回列表
        </button>
      )}

      <div className="rounded-lg border border-neutral-200 bg-white p-4">
        <div className="flex items-start gap-3">
          <div className="h-24 w-16 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100">
            {info.series.member_count > 0 ? (
              <img
                src={seriesPosterUrl(info.series.id)}
                alt={info.series.name}
                className="h-full w-full object-cover"
              />
            ) : (
              <div className="flex h-full w-full items-center justify-center text-neutral-300">
                <Layers className="size-5" />
              </div>
            )}
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-semibold text-neutral-900" title={info.series.name}>
              {info.series.name}
            </p>
            <p className="mt-0.5 text-xs text-neutral-400">系列剧集 · {info.series.member_count} 个成员</p>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {hasSubtitles && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                  字幕 {info.subtitleFiles} 轨 · {formatBytes(info.subtitleBytes)}
                </span>
              )}
              {hasRemux && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                  Remux {info.remuxFiles} 个 · {formatBytes(info.remuxBytes)}
                </span>
              )}
              {hasMemory && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                  记忆 {info.memberPrefs + (info.hasSeriesPrefs ? 1 : 0)} 条
                </span>
              )}
              {!hasSubtitles && !hasMemory && !hasRemux && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-400">无缓存</span>
              )}
            </div>
            {seriesPrefs && (
              <p className="mt-2 truncate text-xs text-neutral-500" title={seriesPrefsSummary(seriesPrefs)}>
                系列共享记忆：{seriesPrefsSummary(seriesPrefs)}
              </p>
            )}
          </div>
        </div>

        <DetailActions
          pregenBusy={pregenBusy}
          activePregen={!!activePregen}
          onPregen={submitPregen}
          hasSubtitles={hasSubtitles}
          onClearSubtitles={() => setDialog('subtitles')}
          hasRemux={hasRemux}
          onClearRemux={() => setDialog('remux')}
          hasMemory={hasMemory}
          onClearPrefs={() => setDialog('prefs')}
          hint="预生成会把成员的内封文本字幕提前提取，播放时秒开"
        />
      </div>

      <div className="rounded-lg border border-neutral-200 bg-white">
        <p className="border-b border-neutral-100 px-4 py-2.5 text-xs font-medium uppercase tracking-wide text-neutral-400">
          成员缓存（{cachedCount}/{members.length}）
        </p>
        {membersQuery.isLoading ? (
          <div className="flex items-center justify-center py-10 text-neutral-400">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : members.length === 0 ? (
          <p className="px-4 py-8 text-center text-sm text-neutral-400">该系列暂无成员。</p>
        ) : (
          <div className="space-y-2 p-3">
            {members.map((m) => (
              <VideoCacheRow
                key={m.video_id}
                videoId={m.video_id}
                episode={String(m.episode_number)}
                title={m.title}
                group={subtitlesByVideo.get(m.video_id)}
                remux={remuxByVideo.get(m.video_id)}
                prefs={prefsByVideo.get(m.video_id)}
                onChanged={onChanged}
              />
            ))}
          </div>
        )}
      </div>

      {dialog === 'subtitles' && (
        <ConfirmDialog
          title="清理字幕缓存"
          description={`将删除「${info.series.name}」以下成员的字幕缓存，下次播放会自动重新提取。`}
          items={subtitleItems}
          confirmLabel="清理字幕"
          onClose={() => setDialog(null)}
          onConfirm={clearSubtitles}
        />
      )}
      {dialog === 'remux' && (
        <ConfirmDialog
          title="清理 Remux 缓存"
          description={`将删除「${info.series.name}」以下成员的 Remux 流拷贝缓存，下次播放时会重新生成。`}
          items={remuxItems}
          confirmLabel="清理 Remux"
          onClose={() => setDialog(null)}
          onConfirm={clearRemux}
        />
      )}
      {dialog === 'prefs' && (
        <ConfirmDialog
          title="清理播放记忆"
          description={`将删除「${info.series.name}」的以下播放选择记忆，播放回到默认音轨/字幕/音量。`}
          items={memoryItems}
          confirmLabel="清理记忆"
          onClose={() => setDialog(null)}
          onConfirm={clearMemory}
        />
      )}
    </div>
  )
}

export function StandaloneCacheDetail({
  item,
  narrow,
  onBack,
  onChanged,
}: {
  item: StandaloneCacheInfo
  narrow: boolean
  onBack?: () => void
  onChanged: () => Promise<void>
}) {
  const [dialog, setDialog] = useState<null | 'subtitles' | 'prefs' | 'remux'>(null)
  const [pregenBusy, setPregenBusy] = useState(false)
  const { notify } = useNotify()
  const jobs = useJobs()
  const activePregen = (jobs.data?.jobs ?? []).find(
    (j) => j.type === 'pregen' && j.target === item.videoId && isActiveJob(j),
  )
  const hasSubtitles = !!item.group
  const hasMemory = !!item.prefs
  const hasRemux = item.remuxFiles > 0

  async function submitPregen() {
    setPregenBusy(true)
    try {
      await pregenSubtitles([item.videoId])
      notify('已提交预生成任务，可在顶部任务栏查看进度')
    } catch (err) {
      notify(err instanceof Error ? err.message : '任务提交失败', 'error')
    } finally {
      setPregenBusy(false)
    }
  }

  async function clearSubtitles() {
    setDialog(null)
    await runClear(() => clearSubtitleCache(item.videoId), `已清空「${item.title}」的字幕缓存`, onChanged, notify)
  }

  async function clearRemux() {
    setDialog(null)
    await runClear(() => clearVideoRemux(item.videoId), `已清理「${item.title}」的 Remux 缓存`, onChanged, notify)
  }

  async function clearMemory() {
    setDialog(null)
    await runClear(() => clearPrefs(item.videoId), `已删除「${item.title}」的播放记忆`, onChanged, notify)
  }

  return (
    <div className="flex flex-col gap-3">
      {narrow && onBack && (
        <button
          onClick={onBack}
          className="flex shrink-0 items-center gap-1 self-start rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-sm text-neutral-600 hover:bg-neutral-100"
        >
          <ArrowLeft className="size-4" /> 返回列表
        </button>
      )}

      <div className="rounded-lg border border-neutral-200 bg-white p-4">
        <div className="flex items-start gap-3">
          <div className="h-24 w-16 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100">
            {item.hasThumb ? (
              <img
                src={coverUrl(item.videoId, true)}
                alt={item.title}
                className="h-full w-full object-cover"
                onError={(e) => {
                  e.currentTarget.style.display = 'none'
                }}
              />
            ) : (
              <div className="flex h-full w-full items-center justify-center text-neutral-300">
                <Layers className="size-5" />
              </div>
            )}
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-semibold text-neutral-900" title={item.title}>
              {item.title}
            </p>
            <p className="mt-0.5 text-xs text-neutral-400">未归组单集</p>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {hasSubtitles && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                  字幕 {item.group!.files.length} 轨 · {formatBytes(item.group!.bytes)}
                </span>
              )}
              {hasRemux && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                  Remux {item.remuxFiles} 个 · {formatBytes(item.remuxBytes)}
                </span>
              )}
              {hasMemory && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">记忆</span>
              )}
              {!hasSubtitles && !hasMemory && !hasRemux && (
                <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-400">无缓存</span>
              )}
            </div>
            {item.prefs && (
              <p className="mt-2 truncate text-xs text-neutral-500" title={prefsSummary(item.prefs)}>
                播放记忆：{prefsSummary(item.prefs)}
              </p>
            )}
          </div>
        </div>

        <DetailActions
          pregenBusy={pregenBusy}
          activePregen={!!activePregen}
          onPregen={submitPregen}
          hasSubtitles={hasSubtitles}
          onClearSubtitles={() => setDialog('subtitles')}
          hasRemux={hasRemux}
          onClearRemux={() => setDialog('remux')}
          hasMemory={hasMemory}
          onClearPrefs={() => setDialog('prefs')}
          hint="预生成会提前提取内封文本字幕，播放时秒开"
        />
      </div>

      {dialog === 'subtitles' && (
        <ConfirmDialog
          title="清理字幕缓存"
          description={`将删除「${item.title}」的 ${item.group?.files.length ?? 0} 个提取字幕，下次播放会自动重新提取。`}
          items={(item.group?.files ?? []).map((f) => ({ label: `轨 ${f.track}`, detail: formatBytes(f.bytes) }))}
          confirmLabel="清理字幕"
          onClose={() => setDialog(null)}
          onConfirm={clearSubtitles}
        />
      )}
      {dialog === 'remux' && (
        <ConfirmDialog
          title="清理 Remux 缓存"
          description={`将删除「${item.title}」的 Remux 流拷贝缓存，下次播放时会重新生成。`}
          items={[{ label: item.title, detail: `${item.remuxFiles} 个 · ${formatBytes(item.remuxBytes)}` }]}
          confirmLabel="清理 Remux"
          onClose={() => setDialog(null)}
          onConfirm={clearRemux}
        />
      )}
      {dialog === 'prefs' && (
        <ConfirmDialog
          title="清理播放记忆"
          description={`将删除「${item.title}」的播放选择记忆，播放回到默认音轨/字幕/音量。`}
          items={[{ label: item.title, detail: item.prefs ? prefsSummary(item.prefs) : undefined }]}
          confirmLabel="清理记忆"
          onClose={() => setDialog(null)}
          onConfirm={clearMemory}
        />
      )}
    </div>
  )
}