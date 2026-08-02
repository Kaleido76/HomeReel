import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Calendar, Check, ImageUp, Pencil, Star, Wand2, X } from 'lucide-react'
import type { ScrapeCandidate, Video } from '../../api/videos'
import { scrapeVideo, updateVideo, uploadVideoCover } from '../../api/videos'
import { fetchSeries } from '../../api/series'

export function VideoMetaPanel({ video, initialTags }: { video: Video; initialTags: string[] }) {
  const queryClient = useQueryClient()
  const [tags, setTags] = useState(initialTags)
  const [tagInput, setTagInput] = useState('')
  const [candidates, setCandidates] = useState<ScrapeCandidate[] | null>(null)
  const [scrapeError, setScrapeError] = useState('')
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleInput, setTitleInput] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['video', video.id] })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
    void queryClient.invalidateQueries({ queryKey: ['series'] })
    void queryClient.invalidateQueries({ queryKey: ['home'] })
  }

  const save = useMutation({
    mutationFn: (patch: Parameters<typeof updateVideo>[1]) => updateVideo(video.id, patch),
    onSuccess: invalidate,
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

  const commitTitle = () => {
    const t = titleInput.trim()
    setEditingTitle(false)
    if (!t || t === title) return
    save.mutate({ title: t })
  }

  const scrape = useMutation({
    mutationFn: (tmdbId?: number) => scrapeVideo(video.id, tmdbId),
    onSuccess: (res) => {
      if (res.candidates) {
        setCandidates(res.candidates)
        setScrapeError('')
      } else {
        setCandidates(null)
        invalidate()
      }
    },
    onError: (err: Error) => {
      setCandidates(null)
      setScrapeError(err.message)
    },
  })

  const uploadCover = useMutation({
    mutationFn: (file: File) => uploadVideoCover(video.id, file),
    onSuccess: invalidate,
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
        <div className="min-w-0 flex-1">
          {editingTitle ? (
            <div className="flex items-center gap-2">
              <input
                value={titleInput}
                onChange={(e) => setTitleInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') commitTitle()
                  if (e.key === 'Escape') setEditingTitle(false)
                }}
                autoFocus
                className="w-full min-w-0 rounded-md border border-neutral-300 bg-white px-2 py-1 text-lg font-medium text-neutral-900 outline-none focus:border-blue-600"
              />
              <button onClick={commitTitle} title="保存" className="shrink-0 rounded p-1 text-blue-600 hover:bg-blue-50">
                <Check className="size-4" />
              </button>
              <button onClick={() => setEditingTitle(false)} title="取消" className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100">
                <X className="size-4" />
              </button>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <h1 className="truncate text-lg font-semibold text-neutral-900" title={title}>
                {title}
              </h1>
              <button
                onClick={() => {
                  setTitleInput(video.title)
                  setEditingTitle(true)
                }}
                title="编辑标题"
                className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-900"
              >
                <Pencil className="size-3.5" />
              </button>
            </div>
          )}
          {video.metadata_source !== 'manual' && (
            <span className="mt-1 inline-block rounded bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500">
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
            className="flex items-center gap-1.5 rounded border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
            title="上传封面"
          >
            <ImageUp className="size-4" /> 封面
          </button>
          <button
            onClick={() => scrape.mutate(undefined)}
            disabled={scrape.isPending}
            className="flex items-center gap-1.5 rounded border border-blue-200 bg-blue-50 px-3 py-1.5 text-sm text-blue-700 hover:bg-blue-100 disabled:opacity-40"
          >
            <Wand2 className="size-4" /> {scrape.isPending ? '搜索中…' : '刮削'}
          </button>
        </div>
      </div>

      <SeriesAssign video={video} save={save.mutate} />

      {meta.length > 0 && (
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-500">
          {video.year && (
            <span className="flex items-center gap-1">
              <Calendar className="size-4" /> {video.year}
            </span>
          )}
          {video.rating ? (
            <span className="flex items-center gap-1">
              <Star className="size-4 fill-neutral-300 text-neutral-400" /> {video.rating.toFixed(1)}
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
        <div className="rounded-md border border-neutral-200 bg-neutral-50 p-3">
          <p className="mb-2 text-sm font-medium text-neutral-700">选择匹配条目</p>
          <div className="grid gap-2 sm:grid-cols-2">
            {candidates.map((c) => (
              <button
                key={c.id}
                onClick={() => scrape.mutate(c.id)}
                disabled={scrape.isPending}
                className="rounded-md border border-neutral-200 bg-white p-2 text-left text-sm hover:border-blue-300 hover:bg-blue-50 disabled:opacity-40"
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
            className="flex items-center gap-1 rounded bg-neutral-100 px-2.5 py-1 text-xs text-neutral-700"
          >
            {t}
            <button onClick={() => removeTag(t)} className="text-neutral-400 hover:text-neutral-700" title="删除标签">
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
          className="w-28 rounded border border-dashed border-neutral-300 bg-transparent px-2.5 py-1 text-xs outline-none focus:border-blue-600"
        />
      </div>

      <p className="truncate font-mono text-xs text-neutral-400" title={video.relative_path}>
        {video.relative_path}
      </p>
    </div>
  )
}

// SeriesAssign lets a standalone video be joined into an existing series (show +
// season) or removed from one. It PATCHes show_id / season_number /
// episode_number, which the backend treats as the authoritative grouping source.
function SeriesAssign({ video, save }: { video: Video; save: (patch: Parameters<typeof updateVideo>[1]) => void }) {
  const seriesQuery = useQuery({ queryKey: ['series'], queryFn: fetchSeries })
  const [showId, setShowId] = useState(video.show_id ?? '')
  const [episode, setEpisode] = useState(video.episode_number ?? 1)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setShowId(video.show_id ?? '')
    setEpisode(video.episode_number ?? 1)
  }, [video.show_id, video.episode_number])

  const series = seriesQuery.data?.series ?? []
  const selected = series.find((s) => s.show_id === showId)

  function apply() {
    if (!showId) {
      save({ show_id: '' })
      return
    }
    if (!selected) return
    setSaving(true)
    save({ show_id: selected.show_id, season_number: selected.season_number, episode_number: episode })
    // saving is transient; the panel refreshes from the invalidated queries
    setTimeout(() => setSaving(false), 0)
  }

  return (
    <div className="rounded-md border border-neutral-200 bg-neutral-50 p-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1.5">
          <span className="shrink-0 text-neutral-500">归属系列</span>
          <select
            value={showId}
            onChange={(e) => setShowId(e.target.value)}
            className="rounded border border-neutral-300 bg-white px-2 py-1 text-sm text-neutral-700 outline-none focus:border-blue-600"
          >
            <option value="">未归入系列</option>
            {series.map((s) => (
              <option key={s.id} value={s.show_id}>
                {s.name}（第 {s.season_number} 季）
              </option>
            ))}
          </select>
        </label>
        {selected && (
          <label className="flex items-center gap-1.5">
            <span className="shrink-0 text-neutral-500">集号</span>
            <input
              type="number"
              min={1}
              value={episode}
              onChange={(e) => setEpisode(Number(e.target.value) || 1)}
              className="w-16 rounded border border-neutral-300 bg-white px-2 py-1 text-sm text-neutral-700 outline-none focus:border-blue-600"
            />
          </label>
        )}
        <button
          onClick={apply}
          disabled={saving || seriesQuery.isLoading}
          className="rounded bg-blue-600 px-2.5 py-1 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          应用
        </button>
      </div>
    </div>
  )
}
