import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eraser, History } from 'lucide-react'
import { clearHistory, fetchHistory } from '../../api/videos'
import { formatDuration } from '../../lib/format'
import { Tooltip } from '../../components/Tooltip'

// PlaybackHistoryCard is the 播放历史 card of the single-episode detail page:
// the resume progress (上次播放到 X / 总时长 · 日期) plus a clear-history action.
// It owns its ['history', id] query and mutation so VideoDetailPane stays a thin
// composition; VideoPlayer invalidates the same key when it exits playback.
export function PlaybackHistoryCard({ videoId, duration }: { videoId: string; duration: number }) {
  const queryClient = useQueryClient()
  const history = useQuery({ queryKey: ['history', videoId], queryFn: () => fetchHistory(videoId) })
  const clearPlayback = useMutation({
    mutationFn: () => clearHistory(videoId),
    onSuccess: () => queryClient.setQueryData(['history', videoId], { history: null }),
  })

  const h = history.data?.history
  const historyPct = duration > 0 && h && h.progress > 0 ? Math.min(100, (h.progress / duration) * 100) : 0

  return (
    <div className="rounded-md border border-neutral-200 bg-white p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-900">
          <History className="size-4 text-neutral-400" />
          播放历史
        </div>
        <Tooltip content={h ? '清空本片进度' : '暂无播放记录'}>
          <button
            onClick={() => clearPlayback.mutate()}
            disabled={clearPlayback.isPending || !h}
            className="flex items-center gap-1.5 rounded border border-neutral-200 px-2.5 py-1 text-xs text-neutral-500 hover:bg-neutral-50 hover:text-neutral-900 disabled:opacity-40"
          >
            <Eraser className="size-3.5" /> 清除历史
          </button>
        </Tooltip>
      </div>
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
    </div>
  )
}
