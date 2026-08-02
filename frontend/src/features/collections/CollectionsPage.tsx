import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderHeart, Loader2, Plus, Trash2 } from 'lucide-react'
import { createCollection, deleteCollection, fetchCollections } from '../../api/collections'

export function CollectionsPage() {
  const queryClient = useQueryClient()
  const collections = useQuery({ queryKey: ['collections'], queryFn: fetchCollections })
  const [name, setName] = useState('')

  const create = useMutation({
    mutationFn: createCollection,
    onSuccess: () => {
      setName('')
      void queryClient.invalidateQueries({ queryKey: ['collections'] })
    },
  })

  const remove = useMutation({
    mutationFn: deleteCollection,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['collections'] }),
  })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900">集合</h1>
      </div>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate(name.trim())
        }}
        className="flex max-w-md items-center gap-2"
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="新建集合名称…"
          className="min-w-0 flex-1 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-400"
        />
        <button
          type="submit"
          disabled={!name.trim() || create.isPending}
          className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3 py-2 text-sm text-white hover:bg-indigo-700 disabled:opacity-40"
        >
          <Plus className="size-4" /> 新建
        </button>
      </form>

      {collections.isLoading && (
        <div className="flex h-40 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      )}

      {collections.data && collections.data.collections.length === 0 && (
        <div className="rounded-xl border border-neutral-200 bg-white px-4 py-12 text-center text-neutral-400">
          暂无集合。创建集合后可把视频按主题整理到一起。
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {collections.data?.collections.map((c) => (
          <div
            key={c.id}
            className="group flex items-center justify-between rounded-xl border border-neutral-200 bg-white px-4 py-3.5 transition-shadow hover:shadow-md"
          >
            <Link to="/collections/$id" params={{ id: c.id }} className="flex min-w-0 items-center gap-2.5">
              <FolderHeart className="size-5 shrink-0 text-indigo-500" />
              <span className="truncate text-sm font-medium text-neutral-800">{c.name}</span>
            </Link>
            <button
              onClick={() => remove.mutate(c.id)}
              disabled={remove.isPending}
              className="shrink-0 rounded-lg p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-40"
              title="删除集合"
            >
              <Trash2 className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
