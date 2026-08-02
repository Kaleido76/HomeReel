import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Calendar, ImageUp, Star, Wand2, X } from 'lucide-react'
import type { ScrapeCandidate, Video } from '../../api/videos'
import { scrapeVideo, updateVideo, uploadVideoCover } from '../../api/videos'

export function VideoMetaPanel({ video, initialTags }: { video: Video; initialTags: string[] }) {
  const queryClient = useQueryClient()
  const [tags, setTags] = useState(initialTags)
  const [tagInput, setTagInput] = useState('')
  const [candidates, setCandidates] = useState<ScrapeCandidate[] | null>(null)
  const [scrapeError, setScrapeError] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const save = useMutation({
    mutationFn: (patch: Parameters<typeof updateVideo>[1]) => updateVideo(video.id, patch),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['video', video.id] }),
  })

  const addTag = () => {
    const t = tagInput.trim()
    if (!t || tags.includes(t)) return
    const next = [...tags, t]
    setTags(next)
    setTagInput('')
    save.mutate({ tags: next })
  }

  const removeTag = (t: string) => {
    const next = tags.filter((x) => x !== t)
    setTags(next)
    save.mutate({ tags: next })
  }

  const scrape = useMutation({
    mutationFn: (tmdbId?: number) => scrapeVideo(video.id, tmdbId),
    onSuccess: (res) => {
      if (res.candidates) {
        setCandidates(res.candidates)
        setScrapeError('')
      } else {
        setCandidates(null)
        void queryClient.invalidateQueries({ queryKey: ['video', video.id] })
      }
    },
    onError: (err: Error) => {
      setCandidates(null)
      setScrapeError(err.message)
    },
  })

  const uploadCover = useMutation({
    mutationFn: (file: File) => uploadVideoCover(video.id, file),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['video', video.id] }),
  })

  const title = video.episode_title || video.title
  const meta = [
    video.year ? `${video.year}` : '',
    video.rating ? `★ ${video.rating.toFixed(1)}` : '',
    video.genre || '',
    video.studio ? `发行：${video.studio}` : '',
    video.cast_text ? `主演：${video.cast_text}` : '',
  ].filter(Boolean)

  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-neutral-900">{title}</h1>
          {video.metadata_source !== 'manual' && (
            <span className="mt-1 inline-block rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500">
              元数据来源：{video.metadata_source}
            </span>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) uploadCover.mutate(f)
              e.target.value = ''
            }}
          />
          <button
            onClick={() => fileRef.current?.click()}
            disabled={uploadCover.isPending}
            className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
            title="上传封面"
          >
            <ImageUp className="size-4" /> 封面
          </button>
          <button
            onClick={() => scrape.mutate(undefined)}
            disabled={scrape.isPending}
            className="flex items-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-3 py-1.5 text-sm text-indigo-700 hover:bg-indigo-100 disabled:opacity-40"
          >
            <Wand2 className="size-4" /> {scrape.isPending ? '搜索中…' : '刮削'}
          </button>
        </div>
      </div>

      {meta.length > 0 && (
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-500">
          {video.year && (
            <span className="flex items-center gap-1">
              <Calendar className="size-4" /> {video.year}
            </span>
          )}
          {video.rating ? (
            <span className="flex items-center gap-1">
              <Star className="size-4 fill-amber-400 text-amber-400" /> {video.rating.toFixed(1)}
            </span>
          ) : null}
          {video.genre || video.studio || video.cast_text ? <span>{meta.slice(2).join(' · ')}</span> : null}
        </p>
      )}

      {video.overview ? (
        <p className="text-sm leading-relaxed text-neutral-600">{video.overview}</p>
      ) : (
        <p className="text-sm text-neutral-400">
          暂无简介。点击「刮削」可从 TMDB 获取，或在文件管理里放置同名 .nfo 文件。
        </p>
      )}

      {scrapeError && <p className="text-sm text-red-600">{scrapeError}</p>}

      {candidates && (
        <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3">
          <p className="mb-2 text-sm font-medium text-neutral-700">选择匹配条目</p>
          <div className="grid gap-2 sm:grid-cols-2">
            {candidates.map((c) => (
              <button
                key={c.id}
                onClick={() => scrape.mutate(c.id)}
                disabled={scrape.isPending}
                className="rounded-lg border border-neutral-200 bg-white p-2 text-left text-sm hover:border-indigo-300 hover:bg-indigo-50 disabled:opacity-40"
              >
                <span className="font-medium text-neutral-800">{c.title}</span>
                {c.year ? <span className="ml-1 text-neutral-400">({c.year})</span> : null}
                {c.overview ? <span className="mt-0.5 block line-clamp-1 text-xs text-neutral-500">{c.overview}</span> : null}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1.5">
        {tags.map((t) => (
          <span
            key={t}
            className="flex items-center gap-1 rounded-full bg-indigo-50 px-2.5 py-1 text-xs text-indigo-700"
          >
            {t}
            <button onClick={() => removeTag(t)} className="text-indigo-400 hover:text-indigo-700" title="删除标签">
              <X className="size-3" />
            </button>
          </span>
        ))}
        <input
          value={tagInput}
          onChange={(e) => setTagInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              addTag()
            }
          }}
          onBlur={addTag}
          placeholder="+ 添加标签"
          className="w-28 rounded-full border border-dashed border-neutral-300 bg-transparent px-2.5 py-1 text-xs outline-none focus:border-indigo-400"
        />
      </div>

      <p className="truncate text-xs text-neutral-400" title={video.relative_path}>
        {video.relative_path}
      </p>
    </div>
  )
}
