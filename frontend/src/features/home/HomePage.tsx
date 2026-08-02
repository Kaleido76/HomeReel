import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Clapperboard, Loader2 } from 'lucide-react'
import { fetchVideos } from '../../api/videos'
import { VideoCard } from '../library/VideoCard'

export function HomePage() {
  const recent = useQuery({
    queryKey: ['videos', 'recent'],
    queryFn: () => fetchVideos({ sort: 'date', order: 'desc', pageSize: 12 }),
  })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900">媒体首页</h1>
        <Link
          to="/library"
          className="flex items-center gap-1.5 rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
        >
          <Clapperboard className="size-4" /> 全部视频
        </Link>
      </div>

      <section className="space-y-3">
        <h2 className="text-sm font-medium text-neutral-500">最近添加</h2>
        {recent.isLoading && (
          <div className="flex h-40 items-center justify-center text-neutral-400">
            <Loader2 className="size-6 animate-spin" />
          </div>
        )}
        {recent.isError && (
          <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-6 text-center text-sm text-red-600">
            {recent.error.message}
          </div>
        )}
        {recent.data && recent.data.videos.length === 0 ? (
          <div className="rounded-xl border border-neutral-200 bg-white px-4 py-12 text-center text-neutral-400">
            暂无视频。请先在「文件管理」配置存储卷并触发扫描。
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {recent.data?.videos.map((v) => (
              <VideoCard key={v.id} video={v} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
