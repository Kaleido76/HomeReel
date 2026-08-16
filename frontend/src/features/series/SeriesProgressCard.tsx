import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Eraser, History } from 'lucide-react'
import { clearSeriesHistory, type SeriesMember } from '../../api/series'
import { Tooltip } from '../../components/Tooltip'

// 已观看判定阈值：观看超过 90% 即视为看完——很多影片/剧集片尾有数分钟报幕，
// 要求接近 100% 会永远判为「未看完」。
const WATCHED_RATIO = 0.9

// SeriesProgressCard is the 观看进度 card of the series detail page: it
// aggregates every member's resume position into one overall progress (已观看
// 时长 / 总时长) plus a watched-episode count, and clears all members' history
// at once (DELETE /api/series/{id}/history). The detail response already carries
// members with progress, so no extra query is needed.
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
    <div className="rounded-md border border-neutral-200 bg-white p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-900">
          <History className="size-4 text-neutral-400" />
          观看进度
        </div>
        <Tooltip content={hasHistory ? '清空本系列进度' : '暂无播放记录'}>
          <button
            onClick={() => clearAll.mutate()}
            disabled={clearAll.isPending || !hasHistory}
            className="flex items-center gap-1.5 rounded border border-neutral-200 px-2.5 py-1 text-xs text-neutral-500 hover:bg-neutral-50 hover:text-neutral-900 disabled:opacity-40"
          >
            <Eraser className="size-3.5" /> 清除全部进度
          </button>
        </Tooltip>
      </div>
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
          {total > 0 && (
            <div className="mt-2 h-1.5 w-full overflow-hidden rounded-sm bg-neutral-100">
              <div className="h-full rounded-sm bg-blue-600" style={{ width: `${pct}%` }} />
            </div>
          )}
        </>
      )}
    </div>
  )
}
