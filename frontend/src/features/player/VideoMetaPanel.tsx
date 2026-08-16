import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Pencil, Star, X } from 'lucide-react'
import type { Video } from '../../api/videos'
import { updateVideo } from '../../api/videos'
import { openFileLocation } from '../../tabs/manager'
import { Tooltip } from '../../components/Tooltip'
import { parentPath } from '../files/path'

export function VideoMetaPanel({ video, initialTags }: { video: Video; initialTags: string[] }) {
  const queryClient = useQueryClient()
  const [tags, setTags] = useState(initialTags)
  const [tagInput, setTagInput] = useState('')
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleInput, setTitleInput] = useState('')

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

  const title = video.episode_title || video.title
  const commitTitle = () => {
    const t = titleInput.trim()
    setEditingTitle(false)
    if (!t || t === title) return
    save.mutate({ title: t })
  }

  const dirPath = video.path ? parentPath(video.path) : null
  // 元信息行文本部分：发行：studio / 主演：cast 显式拼接，不依赖数组下标；
  // rating 带图标单独渲染。
  const metaText = [
    video.studio ? `发行：${video.studio}` : '',
    video.cast_text ? `主演：${video.cast_text}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
  const hasMeta = Boolean(video.rating || metaText)

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
              <Tooltip content="保存">
                <button onClick={commitTitle} className="shrink-0 rounded p-1 text-blue-600 hover:bg-blue-50">
                  <Check className="size-4" />
                </button>
              </Tooltip>
              <Tooltip content="取消">
                <button onClick={() => setEditingTitle(false)} className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100">
                  <X className="size-4" />
                </button>
              </Tooltip>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <h1 className="truncate text-lg font-semibold text-neutral-900" title={title}>
                {title}
              </h1>
              <Tooltip content="编辑标题">
                <button
                  onClick={() => {
                    setTitleInput(video.title)
                    setEditingTitle(true)
                  }}
                  className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-900"
                >
                  <Pencil className="size-3.5" />
                </button>
              </Tooltip>
            </div>
          )}
        </div>
      </div>

      {hasMeta && (
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-500">
          {video.rating ? (
            <span className="flex items-center gap-1">
              <Star className="size-4 fill-neutral-300 text-neutral-400" /> {video.rating.toFixed(1)}
            </span>
          ) : null}
          {metaText && <span>{metaText}</span>}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-1.5">
        {tags.map((t) => (
          <span
            key={t}
            className="flex items-center gap-1 rounded bg-neutral-100 px-2.5 py-1 text-xs text-neutral-700"
          >
            {t}
            <Tooltip content="删除标签">
              <button onClick={() => removeTag(t)} className="text-neutral-400 hover:text-neutral-700">
                <X className="size-3" />
              </button>
            </Tooltip>
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

      {dirPath ? (
        <button
          onClick={() => openFileLocation(dirPath)}
          title={`在文件页定位所在目录：${video.path}`}
          className="inline-block max-w-full truncate font-mono text-xs text-neutral-400 transition-colors hover:text-blue-600 hover:underline"
        >
          {video.relative_path}
        </button>
      ) : (
        <p className="truncate font-mono text-xs text-neutral-400" title={video.relative_path}>
          {video.relative_path}
        </p>
      )}
    </div>
  )
}

