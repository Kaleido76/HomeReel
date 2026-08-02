import { useQuery } from '@tanstack/react-query'
import { Clapperboard, Loader2 } from 'lucide-react'
import { fetchHome } from '../../api/home'
import { VideoCard } from '../library/VideoCard'
import { MediaGrid } from '../library/MediaGrid'
import { openLibrary } from '../../tabs/manager'

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
          className="flex items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
        >
          <Clapperboard className="size-4" /> 全部视频
        </button>
      </div>

      {continueWatching.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-sm font-medium text-neutral-500">继续观看</h2>
          <MediaGrid>
            {continueWatching.map((v) => (
              <VideoCard key={v.id} video={v} />
            ))}
          </MediaGrid>
        </section>
      )}

      <section className="space-y-3">
        <h2 className="text-sm font-medium text-neutral-500">最近添加</h2>
        {recent.length === 0 ? (
          <div className="rounded-xl border border-neutral-200 bg-white px-4 py-12 text-center text-neutral-400">
            暂无视频。请先在「文件管理」配置存储卷并触发扫描。
          </div>
        ) : (
          <MediaGrid>
            {recent.map((v) => (
              <VideoCard key={v.id} video={v} />
            ))}
          </MediaGrid>
        )}
      </section>
    </div>
  )
}
