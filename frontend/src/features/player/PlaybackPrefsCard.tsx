import { SlidersHorizontal } from 'lucide-react'
import { HistorySection } from '../../components/HistorySection'

// PlaybackPrefsCard is the 配置缓存 小节 of the detail pages (rendered inside the
// merged PlaybackHistoryCard): the remembered playback selections (audio track /
// subtitle / volume, ADR-006 player prefs) as plain info rows plus an icon-only
// clear action (label in the ToolTip). The caller resolves the display strings — a
// series detail passes the raw shared track names, a single-episode detail the
// effective values (which may come from the series). Clearing resets the effective
// record so the next play falls back to the browser defaults (and, for a series
// member, to its own per-video record).
export function PlaybackPrefsCard({
  audio,
  subtitle,
  volume,
  note,
  hasPrefs,
  clearing,
  onClear,
}: {
  audio?: string
  subtitle?: string
  volume?: string
  note?: string
  hasPrefs: boolean
  clearing?: boolean
  onClear: () => void
}) {
  return (
    <HistorySection
      icon={<SlidersHorizontal className="size-4 text-neutral-400" />}
      title="配置缓存"
      note={note}
      clearTip={hasPrefs ? '清除配置缓存，回到默认音轨/字幕/音量' : '暂无配置缓存'}
      has={hasPrefs}
      clearing={clearing}
      onClear={onClear}
    >
      {!hasPrefs ? (
        <p className="mt-2 text-sm text-neutral-500">尚未记录配置缓存。</p>
      ) : (
        <dl className="mt-2 space-y-1 text-sm">
          <PrefsRow label="音轨" value={audio} />
          <PrefsRow label="字幕" value={subtitle} />
          <PrefsRow label="音量" value={volume} />
        </dl>
      )}
    </HistorySection>
  )
}

function PrefsRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <dt className="text-neutral-400">{label}</dt>
      <dd className="text-right text-neutral-800">{value ?? '未记忆'}</dd>
    </div>
  )
}
