import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { Loader2, Search } from 'lucide-react'
import { searchVideos } from '../../api/videos'
import { VideoCard } from '../library/VideoCard'
import { MediaGrid } from '../library/MediaGrid'

// The search query lives in the URL (?q=) so the submitted term and its results
// survive refresh and in-tab back/forward.
export function SearchPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const search = (location.search ?? {}) as { q?: string }
  const [input, setInput] = useState(search.q ?? '')
  const submitted = search.q ?? ''

  const results = useQuery({
    queryKey: ['search', submitted],
    queryFn: () => searchVideos(submitted),
    enabled: submitted !== '',
  })

  function submit() {
    navigate({ to: '/search', search: { q: input.trim() } })
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold text-neutral-900">搜索</h1>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="flex items-center gap-2 rounded-lg border border-neutral-200 bg-white px-3 py-2.5"
      >
        <Search className="size-5 shrink-0 text-neutral-400" />
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="搜索标题、剧名、标签…"
          autoFocus
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
        />
        <button
          type="submit"
          disabled={!input.trim()}
          className="rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          搜索
        </button>
      </form>

      {submitted === '' ? (
        <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          输入关键词搜索视频，支持标题、文件名、标签与剧名。
        </div>
      ) : results.isLoading ? (
        <div className="flex h-40 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      ) : results.isError ? (
        <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
          {results.error.message}
        </div>
      ) : (
        <>
          {results.data && results.data.videos.length === 0 ? (
            <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
              没有找到与「{submitted}」匹配的视频
            </div>
          ) : (
            <MediaGrid>
              {results.data?.videos.map((v) => (
                <VideoCard key={v.id} video={v} />
              ))}
            </MediaGrid>
          )}
        </>
      )}
    </div>
  )
}
