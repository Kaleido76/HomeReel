import { useQuery } from '@tanstack/react-query'
import { Clapperboard, Loader2, Play } from 'lucide-react'
import { fetchHome } from '../../api/home'
import { coverUrl } from '../../api/videos'
import { VideoCard } from '../library/VideoCard'
import { formatDuration } from '../../lib/format'
import { openLibrary, openVideo } from '../../tabs/manager'

export function HomePage() {
  const home = useQuery({ queryKey: ['home'], queryFn: fetchHome })

  if (home.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (home.isError) {
    return (
      <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {home.error.message}
      </div>
    )
  }

  const { continue_watching: continueWatching, recent } = home.data ?? {
    continue_watching: [],
    recent: [],
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900">媒体首页</h1>
        <button
          onClick={openLibrary}
          className="flex items-center gap-1.5 rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
        >
          <Clapperboard className="size-4" /> 全部视频
        </button>
      </div>

      {continueWatching.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-sm font-medium text-neutral-500">继续观看</h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {continueWatching.map((v) => (
              <ResumeCard key={v.id} id={v.id} title={v.title} thumb={v.thumb_path} duration={v.duration} />
            ))}
          </div>
        </section>
      )}

      <section className="space-y-3">
        <h2 className="text-sm font-medium text-neutral-500">最近添加</h2>
        {recent.length === 0 ? (
          <div className="rounded-xl border border-neutral-200 bg-white px-4 py-12 text-center text-neutral-400">
            暂无视频。请先在「文件管理」配置存储卷并触发扫描。
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {recent.map((v) => (
              <VideoCard key={v.id} video={v} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

function ResumeCard({ id, title, thumb, duration }: { id: string; title: string; thumb: string; duration: number }) {
  return (
    <button
      onClick={() => openVideo(id)}
      className="group relative overflow-hidden rounded-xl border border-neutral-200 bg-white text-left"
    >
      <div className="relative aspect-video w-full overflow-hidden bg-neutral-100">
        {thumb ? (
          <img src={coverUrl(id, true)} alt={title} loading="lazy" className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-neutral-300">
            <Play className="size-10" />
          </div>
        )}
        <span className="absolute inset-0 flex items-center justify-center bg-black/0 opacity-0 transition-opacity group-hover:bg-black/20 group-hover:opacity-100">
          <Play className="size-10 text-white drop-shadow" />
        </span>
        {duration > 0 && (
          <span className="absolute bottom-1.5 right-1.5 rounded bg-black/60 px-1.5 py-0.5 text-xs text-white">
            {formatDuration(duration)}
          </span>
        )}
      </div>
      <p className="truncate px-3 py-2 text-sm font-medium text-neutral-800" title={title}>
        {title}
      </p>
    </button>
  )
}
