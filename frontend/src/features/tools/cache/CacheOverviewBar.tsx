import type { CacheOverview } from '../../../api/cache'
import { formatBytes } from '../../../lib/format'

// CacheOverviewBar is the page's compact state strip. The total includes the
// (often dominant) remux cache, so a one-line breakdown below the grid states
// where the bytes actually sit — otherwise a 6 GB total next to a few KB of
// subtitle files reads as a contradiction.
export function CacheOverviewBar({ overview }: { overview?: CacheOverview }) {
  const o = overview
  const totalBytes = o
    ? o.orphans.cover.bytes + o.orphans.thumb.bytes + o.orphans.subtitle.bytes + o.orphans.remux.bytes
    : 0
  const subtitleFiles = o?.orphans.subtitle.files ?? 0
  const subtitleBytes = o?.orphans.subtitle.bytes ?? 0
  const prefsCount = o ? o.prefs.length + o.series_prefs.length : 0
  const coverThumbFiles = o ? o.orphans.cover.files + o.orphans.thumb.files : 0
  const coverThumbBytes = o ? o.orphans.cover.bytes + o.orphans.thumb.bytes : 0
  const remuxFiles = o?.orphans.remux.files ?? 0
  const remuxBytes = o?.orphans.remux.bytes ?? 0

  const cells = [
    { label: '缓存总占用', value: formatBytes(totalBytes) },
    { label: '字幕缓存', value: `${subtitleFiles} 轨 · ${formatBytes(subtitleBytes)}` },
    { label: '播放记忆', value: `${prefsCount} 条` },
    { label: '封面与缩略图', value: `${coverThumbFiles} 个 · ${formatBytes(coverThumbBytes)}` },
  ]

  return (
    <div className="shrink-0 rounded-lg border border-neutral-200 bg-white">
      <div className="grid grid-cols-2 gap-px bg-neutral-200 sm:grid-cols-4">
        {cells.map((c) => (
          <div key={c.label} className="min-w-0 bg-white px-4 py-3">
            <p className="truncate text-xs text-neutral-400">{c.label}</p>
            <p className="mt-0.5 truncate text-sm font-medium text-neutral-800">{c.value}</p>
          </div>
        ))}
      </div>
      {remuxBytes > 0 && (
        <p className="border-t border-neutral-100 px-4 py-1.5 text-[11px] text-neutral-400">
          其中 Remux 缓存 {remuxFiles} 个 · {formatBytes(remuxBytes)}（整片流拷贝，播放时自动生成）
        </p>
      )}
    </div>
  )
}