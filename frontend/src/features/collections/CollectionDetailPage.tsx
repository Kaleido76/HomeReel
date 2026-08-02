import { useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Loader2, Pencil, Trash2 } from 'lucide-react'
import type { Storage } from '../../api/storages'
import { fetchStorages } from '../../api/storages'
import { fetchCollections, removeFromCollection, renameCollection } from '../../api/collections'
import { fetchCollectionVideos } from '../../api/collections'
import { VideoCard } from '../library/VideoCard'
import { useMemo } from 'react'

export function CollectionDetailPage() {
  const { id } = useParams({ from: '/collections/$id' })
  const queryClient = useQueryClient()
  const collection = useQuery({ queryKey: ['collections', id], queryFn: () => fetchCollections() })
  const videos = useQuery({ queryKey: ['collections', id, 'videos'], queryFn: () => fetchCollectionVideos(id) })
  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState('')

  const storageById = useMemo(() => {
    const map = new Map<string, Storage>()
    for (const s of storages.data?.storages ?? []) map.set(s.id, s)
    return map
  }, [storages.data])

  const info = collection.data?.collections.find((c) => c.id === id)

  const rename = useMutation({
    mutationFn: (n: string) => renameCollection(id, n),
    onSuccess: () => {
      setEditing(false)
      void queryClient.invalidateQueries({ queryKey: ['collections'] })
    },
  })

  const remove = useMutation({
    mutationFn: (videoId: string) => removeFromCollection(id, videoId),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['collections', id, 'videos'] }),
  })

  if (videos.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <Link to="/collections" className="flex items-center gap-1.5 text-sm text-neutral-500 hover:text-neutral-900">
        <ArrowLeft className="size-4" /> 返回集合
      </Link>

      <div className="flex items-center justify-between">
        {editing ? (
          <form
            onSubmit={(e) => {
              e.preventDefault()
              if (name.trim()) rename.mutate(name.trim())
            }}
            className="flex items-center gap-2"
          >
            <input
              autoFocus
              defaultValue={info?.name}
              onChange={(e) => setName(e.target.value)}
              className="rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-lg font-semibold outline-none focus:border-indigo-400"
            />
            <button
              type="submit"
              className="rounded-lg bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700"
            >
              保存
            </button>
          </form>
        ) : (
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold text-neutral-900">{info?.name ?? '集合'}</h1>
            <button
              onClick={() => {
                setName(info?.name ?? '')
                setEditing(true)
              }}
              className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700"
              title="重命名"
            >
              <Pencil className="size-4" />
            </button>
          </div>
        )}
        <span className="text-sm text-neutral-400">{videos.data?.videos.length ?? 0} 个视频</span>
      </div>

      {videos.data && videos.data.videos.length === 0 ? (
        <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          集合为空。在播放页或视频卡片上点击「加入集合」即可添加。
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {videos.data?.videos.map((v) => (
            <div key={v.id} className="group relative">
              <VideoCard video={v} storage={storageById.get(v.storage_id)} />
              <button
                onClick={() => remove.mutate(v.id)}
                disabled={remove.isPending}
                className="absolute top-2 left-2 rounded-full bg-black/50 p-1.5 text-white opacity-0 transition-opacity hover:bg-red-600 group-hover:opacity-100 disabled:opacity-40"
                title="移出集合"
              >
                <Trash2 className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
