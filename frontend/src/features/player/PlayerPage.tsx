import { Link, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Loader2, PlayCircle } from 'lucide-react'
import { fetchStorages } from '../../api/storages'
import { fetchVideo } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import { VideoPlayer } from './VideoPlayer'

export function PlayerPage() {
  const { id } = useParams({ from: '/library/video/$id' })
  const detail = useQuery({
    queryKey: ['video', id],
    queryFn: () => fetchVideo(id),
  })
  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })

  if (detail.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError) {
    return (
      <div className="space-y-4">
        <Link to="/library" className="flex items-center gap-1.5 text-sm text-neutral-500 hover:text-neutral-900">
          <ArrowLeft className="size-4" /> 返回视频库
        </Link>
        <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
          {detail.error.message}
        </div>
      </div>
    )
  }

  if (!detail.data) return null
  const video = detail.data.video
  const directPlayable = detail.data.direct_playable
  const hlsEnabled = detail.data.hls_enabled
  const storage = storages.data?.storages.find((s) => s.id === video.storage_id)
  const offline = storage !== undefined && !storage.available

  return (
    <div className="space-y-4">
      <Link to="/library" className="flex items-center gap-1.5 text-sm text-neutral-500 hover:text-neutral-900">
        <ArrowLeft className="size-4" /> 返回视频库
      </Link>

      {offline ? (
        <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          <PlayCircle className="mx-auto mb-2 size-10" />
          <p className="mb-1 text-neutral-700">「{video.title}」所在存储卷当前离线</p>
          <p className="text-sm">请检查设备连接，插回后即可播放</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-neutral-200 bg-black">
          <VideoPlayer video={video} directPlayable={directPlayable} hlsEnabled={hlsEnabled} />
        </div>
      )}

      <div className="rounded-xl border border-neutral-200 bg-white px-4 py-3">
        <h1 className="text-lg font-semibold text-neutral-900">{video.title}</h1>
        <p className="mt-1 text-sm text-neutral-500">
          {video.width && video.height ? `${video.width}×${video.height} · ` : ''}
          {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
          {formatBytes(video.size)} · {video.container?.toUpperCase()}
        </p>
        <p className="mt-1 truncate text-xs text-neutral-400" title={video.relative_path}>
          {video.relative_path}
        </p>
      </div>
    </div>
  )
}
