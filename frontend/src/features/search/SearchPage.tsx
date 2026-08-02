import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, Search } from 'lucide-react'
import type { Storage } from '../../api/storages'
import { fetchStorages } from '../../api/storages'
import { searchVideos } from '../../api/videos'
import { VideoCard } from '../library/VideoCard'
import { useMemo } from 'react'

export function SearchPage() {
  const [q, setQ] = useState('')
  const [submitted, setSubmitted] = useState('')

  const results = useQuery({
    queryKey: ['search', submitted],
    queryFn: () => searchVideos(submitted),
    enabled: submitted !== '',
  })

  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })
  const storageById = useMemo(() => {
    const map = new Map<string, Storage>()
    for (const s of storages.data?.storages ?? []) map.set(s.id, s)
    return map
  }, [storages.data])

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold text-neutral-900">搜索</h1>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          setSubmitted(q.trim())
        }}
        className="flex items-center gap-2 rounded-lg border border-neutral-200 bg-white px-3 py-2.5"
      >
        <Search className="size-5 shrink-0 text-neutral-400" />
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="搜索标题、剧名、标签、简介…"
          autoFocus
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
        />
        <button
          type="submit"
          disabled={!q.trim()}
          className="rounded-lg bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-40"
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
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
              {results.data?.videos.map((v) => (
                <VideoCard key={v.id} video={v} storage={storageById.get(v.storage_id)} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
