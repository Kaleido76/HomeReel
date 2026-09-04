import { useState } from 'react'
import { Eraser, Trash2 } from 'lucide-react'
import type { CacheRemuxGroup, PlaybackPrefsCacheEntry, SubtitleCacheGroup } from '../../../api/cache'
import { clearPrefs, clearSubtitleCache } from '../../../api/cache'
import { useNotify } from '../../../components/NotificationProvider'
import { formatBytes } from '../../../lib/format'
import { prefsSummary } from './model'
import { ConfirmDialog } from './ConfirmDialog'

// VideoCacheRow renders one video's cache state inside a detail pane. Subtitle
// tracks are managed as a bundle (clearing one video's subtitles is the unit
// of action), so the row offers at most 清理字幕 / 清理记忆 and rows without any
// cache are visually dimmed with a「无缓存」tag. Series members and standalone
// videos share it.
export function VideoCacheRow({
  videoId,
  episode,
  title,
  group,
  prefs,
  remux,
  onChanged,
}: {
  videoId: string
  episode?: string
  title: string
  group?: SubtitleCacheGroup
  prefs?: PlaybackPrefsCacheEntry
  remux?: CacheRemuxGroup
  onChanged: () => Promise<void>
}) {
  const [dialog, setDialog] = useState<null | 'subtitles' | 'prefs'>(null)
  const { notify } = useNotify()
  const hasCache = !!group || !!prefs || !!remux
  const parts: string[] = []
  if (group) parts.push(`${group.files.length} 轨 · ${formatBytes(group.bytes)}`)
  if (prefs) parts.push(prefsSummary(prefs))
  if (remux) parts.push(`Remux ${remux.files} 个 · ${formatBytes(remux.bytes)}`)
  const summary = parts.length > 0 ? parts.join(' · ') : '无缓存'

  async function doClear(fn: () => Promise<{ cleared: number }>, label: string) {
    try {
      const { cleared } = await fn()
      await onChanged()
      notify(`${label}，已清理 ${cleared} 项`)
    } catch (err) {
      notify(err instanceof Error ? err.message : '操作失败', 'error')
    }
  }

  return (
    <div className={`rounded-lg border bg-white ${hasCache ? 'border-neutral-200' : 'border-neutral-100 opacity-50'}`}>
      <div className="flex items-center justify-between gap-3 p-3">
        <div className="flex min-w-0 items-center gap-2">
          {episode !== undefined && (
            <span className="w-8 shrink-0 text-right font-mono text-xs text-neutral-400">{episode}</span>
          )}
          <div className="min-w-0">
            <p className={`truncate text-sm font-medium ${hasCache ? 'text-neutral-800' : 'text-neutral-500'}`}>
              {title}
            </p>
            <p className={`truncate text-xs ${hasCache ? 'text-neutral-500' : 'text-neutral-400'}`}>{summary}</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {group && (
            <button
              onClick={() => setDialog('subtitles')}
              className="flex items-center gap-1 rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-700 hover:bg-neutral-100"
            >
              <Eraser className="size-3" /> 清理字幕
            </button>
          )}
          {prefs && (
            <button
              onClick={() => setDialog('prefs')}
              className="flex items-center gap-1 rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-700 hover:bg-neutral-100"
            >
              <Trash2 className="size-3" /> 清理记忆
            </button>
          )}
        </div>
      </div>

      {dialog === 'subtitles' && (
        <ConfirmDialog
          title="清理字幕缓存"
          description={`将删除「${title}」的 ${group?.files.length ?? 0} 个提取字幕，下次播放会自动重新提取。`}
          items={(group?.files ?? []).map((f) => ({
            label: `轨 ${f.track}`,
            detail: formatBytes(f.bytes),
          }))}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            doClear(() => clearSubtitleCache(videoId), `已清空「${title}」的字幕缓存`).then(() => setDialog(null))
          }
        />
      )}
      {dialog === 'prefs' && (
        <ConfirmDialog
          title="清理播放记忆"
          description={`将删除「${title}」的播放选择记忆，播放回到默认音轨/字幕/音量。`}
          items={[{ label: title, detail: prefs ? prefsSummary(prefs) : undefined }]}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            doClear(() => clearPrefs(videoId), `已删除「${title}」的播放记忆`).then(() => setDialog(null))
          }
        />
      )}
    </div>
  )
}