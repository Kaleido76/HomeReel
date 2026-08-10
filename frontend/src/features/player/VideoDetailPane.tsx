import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Play, RefreshCw, Trash2 } from 'lucide-react'
import { ApiError } from '../../api/client'
import { deleteVideo, fetchVideo, syncVideo } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import { canPlay } from '../../lib/playability'
import { openFormat } from '../../tabs/manager'
import { VideoMetaPanel } from '../player/VideoMetaPanel'

const MP4_FAMILY = new Set(['mp4', 'm4v', 'mov', 'qt', '3gp', '3g2'])
function isMp4Family(container?: string): boolean {
  return MP4_FAMILY.has(container?.toLowerCase() ?? '')
}

// VideoDetailPane is the single-video detail shown in the middle column. It is
// deliberately compact: a play action, the editable metadata panel and a
// technical summary line. The heavy player lives in the right-hand column
// (PlayerPane), which opens on play.
//
// playHref is resolved by the caller from the column stack: a detail opened
// inside a series plays within the series (/series/:id/play/:videoId), a
// standalone detail plays via /library/video/:id/play.
//
// The detail page also checks the source file on demand (单集详情检查): a file
// that was renamed/moved (found by file_id) shows a sync warning; a file that
// is gone shows a Not-Found warning with the option to remove the record (the
// removal only deletes metadata, never the file).
export function VideoDetailPane({ videoId, playHref }: { videoId: string; playHref: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const detail = useQuery({ queryKey: ['video', videoId], queryFn: () => fetchVideo(videoId) })

  if (detail.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {detail.error.message}
      </div>
    )
  }

  if (!detail.data) return null
  const video = detail.data.video
  const status = detail.data.source_status
  const playable = canPlay(video, detail.data.direct_playable)
  const openConvert = () =>
    openFormat([{ path: video.path, name: video.path.split(/[\\/]/).pop() ?? video.path, is_dir: false }])

  async function doSync() {
    setError('')
    try {
      await syncVideo(video.id)
      queryClient.invalidateQueries({ queryKey: ['video', video.id] })
      queryClient.invalidateQueries({ queryKey: ['videos'] })
      queryClient.invalidateQueries({ queryKey: ['series'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '同步失败')
    }
  }

  async function remove() {
    if (!window.confirm('该单集的源文件已不存在。移除将删除其已入库元数据，磁盘文件不受影响。')) return
    setError('')
    try {
      await deleteVideo(video.id)
      queryClient.invalidateQueries({ queryKey: ['videos'] })
      queryClient.invalidateQueries({ queryKey: ['series'] })
      navigate({ to: '/library' })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '移除失败')
    }
  }

  return (
    <div className="space-y-4 p-4 sm:p-5">
      {playable ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm font-medium text-neutral-500">播放</span>
          <button
            onClick={() => navigate({ href: playHref })}
            className="flex items-center gap-1.5 rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
          >
            <Play className="size-4" /> 播放
          </button>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 p-3">
          <div className="min-w-0">
            <p className="text-sm font-medium text-amber-800">
              该格式不支持直接播放（{video.container?.toUpperCase() || '未知容器'} · {video.codec || '未知编码'}）
            </p>
            <p className="mt-0.5 text-xs text-amber-700">建议用格式工厂转换为 MP4 后再观看，转换不会修改原文件。</p>
          </div>
          <button
            onClick={openConvert}
            className="flex shrink-0 items-center gap-1.5 rounded bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700"
          >
            <Play className="size-4" /> 格式工厂转换
          </button>
        </div>
      )}

      {status === 'moved' && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3">
          <p className="text-sm text-amber-800">
            源文件已改名或移动（{detail.data.new_path ? `现位于 ${detail.data.new_path}` : '位置已变化'}）。
            点击同步以更新库中的路径与归属。
          </p>
          <button
            onClick={() => void doSync()}
            className="mt-2 flex items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700"
          >
            <RefreshCw className="size-3.5" /> 同步
          </button>
        </div>
      )}

      {status === 'missing' && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3">
          <p className="text-sm text-red-700">源文件不存在（可能已被移动或删除）。</p>
          <button
            onClick={() => void remove()}
            className="mt-2 flex items-center gap-1.5 rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700"
          >
            <Trash2 className="size-3.5" /> 移除这个单集
          </button>
        </div>
      )}

      <div className="rounded-md border border-neutral-200 bg-white p-4">
        <VideoMetaPanel video={video} initialTags={detail.data.tags ?? []} />
      </div>

      <div className="rounded-md border border-neutral-200 bg-white px-4 py-3">
        <p className="text-sm text-neutral-500">
          {video.width && video.height ? `${video.width}×${video.height} · ` : ''}
          {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
          {formatBytes(video.size)} · {video.container?.toUpperCase()}
          {video.audio_codec ? ` · 音频 ${video.audio_codec}` : ''}
          <span className={playable ? 'ml-2 text-emerald-600' : 'ml-2 text-amber-600'}>
            {playable ? '可直连播放' : '不可直接播放'}
          </span>
          {isMp4Family(video.container) && !video.faststart && (
            <span className="ml-2 text-amber-600" title="moov 位于文件尾部，浏览器需缓冲后才能拖动进度">
              非快速启动
            </span>
          )}
          {detail.data.series_id ? (
            <Link to="/series/$id" params={{ id: detail.data.series_id }} className="ml-2 text-blue-600 hover:underline">
              所属系列 →
            </Link>
          ) : null}
        </p>
      </div>

      {error && <p className="text-xs text-red-600">{error}</p>}
    </div>
  )
}
