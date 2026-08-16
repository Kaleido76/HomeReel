import { useMutation, useQueryClient } from '@tanstack/react-query'
import { History } from 'lucide-react'
import { clearSeriesHistory, type SeriesMember } from '../../api/series'
import { HistorySection } from '../../components/HistorySection'
import { ProgressBar } from '../../components/ProgressBar'

// 已观看判定阈值：观看超过 90% 即视为看完——很多影片/剧集片尾有数分钟报幕，
// 要求接近 100% 会永远判为「未看完」。
const WATCHED_RATIO = 0.9

// SeriesProgressCard is the 观看进度 小节 of the series detail page (rendered
// inside the merged PlaybackHistoryCard): it aggregates every member's resume
// position into one overall progress (已观看时长 / 总时长) plus a watched-episode
// count, with an icon-only clear action (DELETE /api/series/{id}/history). The
// detail response already carries members with progress, so no extra query is
// needed.
export function SeriesProgressCard({ seriesId, members }: { seriesId: string; members: SeriesMember[] }) {
  const queryClient = useQueryClient()
  const clearAll = useMutation({
    mutationFn: () => clearSeriesHistory(seriesId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      void queryClient.invalidateQueries({ queryKey: ['home'] })
    },
  })

  const total = members.reduce((s, m) => s + Math.max(0, m.duration), 0)
  const consumed = members.reduce((s, m) => s + Math.min(Math.max(0, m.progress), Math.max(0, m.duration)), 0)
  const pct = total > 0 ? Math.min(100, (consumed / total) * 100) : 0
  const watched = members.filter((m) => m.duration > 0 && m.progress > m.duration * WATCHED_RATIO).length
  const hasHistory = members.some((m) => m.progress > 0)

  return (
    <HistorySection
      icon={<History className="size-4 text-neutral-400" />}
      title="观看进度"
      clearTip={hasHistory ? '清空本系列进度' : '暂无播放记录'}
      has={hasHistory}
      clearing={clearAll.isPending}
      onClear={() => clearAll.mutate()}
    >
      {members.length === 0 ? (
        <p className="mt-2 text-sm text-neutral-500">该系列暂无成员。</p>
      ) : !hasHistory ? (
        <p className="mt-2 text-sm text-neutral-500">尚未播放过。</p>
      ) : (
        <>
          <p className="mt-2 text-sm text-neutral-500">
            已观看 {watched} / {members.length} 集
            {total > 0 ? ` · 整体进度 ${Math.round(pct)}%` : ''}
          </p>
          {total > 0 && <ProgressBar value={pct} className="mt-2 h-1.5 w-full" />}
        </>
      )}
    </HistorySection>
  )
}
