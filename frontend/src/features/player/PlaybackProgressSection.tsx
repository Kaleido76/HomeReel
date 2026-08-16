import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History } from 'lucide-react'
import { clearHistory, fetchHistory } from '../../api/videos'
import { HistorySection } from '../../components/HistorySection'
import { formatDuration } from '../../lib/format'

// PlaybackProgressSection is the 观看进度 小节 of the single-episode detail page
// (rendered inside the merged PlaybackHistoryCard): the resume progress (上次播放
// 到 X / 总时长 · 日期) plus an icon-only clear-history action. It owns its
// ['history', id] query and mutation so VideoDetailPane stays a thin composition;
// VideoPlayer invalidates the same key when it exits playback.
export function PlaybackProgressSection({ videoId, duration }: { videoId: string; duration: number }) {
  const queryClient = useQueryClient()
  const history = useQuery({ queryKey: ['history', videoId], queryFn: () => fetchHistory(videoId) })
  const clearPlayback = useMutation({
    mutationFn: () => clearHistory(videoId),
    onSuccess: () => queryClient.setQueryData(['history', videoId], { history: null }),
  })

  const h = history.data?.history
  const historyPct = duration > 0 && h && h.progress > 0 ? Math.min(100, (h.progress / duration) * 100) : 0

  return (
    <HistorySection
      icon={<History className="size-4 text-neutral-400" />}
      title="观看进度"
      clearTip={h ? '清空本片进度' : '暂无播放记录'}
      has={!!h}
      clearing={clearPlayback.isPending}
      onClear={() => clearPlayback.mutate()}
    >
      {!h ? (
        <p className="mt-2 text-sm text-neutral-500">尚未播放过。</p>
      ) : h.progress > 0 ? (
        <>
          <p className="mt-2 text-sm text-neutral-500">
            上次播放到 {formatDuration(h.progress)}
            {duration > 0 ? ` / ${formatDuration(duration)}` : ''}
            {h.updated_at ? ` · ${h.updated_at.slice(0, 10)}` : ''}
          </p>
          {duration > 0 && (
            <div className="mt-2 h-1.5 w-full overflow-hidden rounded-sm bg-neutral-100">
              <div className="h-full rounded-sm bg-blue-600" style={{ width: `${historyPct}%` }} />
            </div>
          )}
        </>
      ) : (
        <p className="mt-2 text-sm text-neutral-500">上次已播放完毕，将从片头开始。</p>
      )}
    </HistorySection>
  )
}
