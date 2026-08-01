import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { HardDrive, Loader2, Network, Plus, RefreshCw, Trash2, Usb, X } from 'lucide-react'
import { ApiError } from '../../api/client'
import {
  createStorage,
  deleteStorage,
  refreshStorage,
  type Storage,
  type StorageType,
} from '../../api/storages'

const typeLabels: Record<StorageType, string> = {
  internal: '内置',
  external: '外接',
  network: '网络',
}

const typeIcons: Record<StorageType, typeof HardDrive> = {
  internal: HardDrive,
  external: Usb,
  network: Network,
}

export function StorageSidebar({
  storages,
  isLoading,
  selectedId,
  onSelect,
}: {
  storages: Storage[]
  isLoading: boolean
  selectedId: string
  onSelect: (id: string) => void
}) {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ name: '', type: 'internal' as StorageType, root_path: '', device_id: '' })
  const [formError, setFormError] = useState('')

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['storages'] })

  const createMut = useMutation({
    mutationFn: createStorage,
    onSuccess: (data) => {
      invalidate()
      setAdding(false)
      setForm({ name: '', type: 'internal', root_path: '', device_id: '' })
      setFormError('')
      onSelect(data.storage.id)
    },
    onError: (err) => setFormError(err instanceof ApiError ? err.message : '创建失败'),
  })

  const deleteMut = useMutation({
    mutationFn: deleteStorage,
    onSuccess: invalidate,
  })

  const refreshMut = useMutation({
    mutationFn: refreshStorage,
    onSuccess: invalidate,
  })

  function remove(s: Storage) {
    if (window.confirm(`移除存储卷「${s.name}」？磁盘文件不会被删除。`)) {
      deleteMut.mutate(s.id)
    }
  }

  function renderGroup(items: Storage[], label: string) {
    if (items.length === 0) return null
    return (
      <div className="space-y-1">
        <p className="px-2 text-xs font-medium uppercase tracking-wide text-neutral-400">{label}</p>
        {items.map((s) => {
          const Icon = typeIcons[s.type]
          return (
            <div
              key={s.id}
              onClick={() => onSelect(s.id)}
              className={`group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors ${
                s.id === selectedId ? 'bg-indigo-50 text-indigo-700' : 'text-neutral-700 hover:bg-neutral-100'
              }`}
            >
              <Icon className="size-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{s.name}</span>
              <span
                title={s.available ? `${typeLabels[s.type]} · 在线` : `${typeLabels[s.type]} · 离线`}
                className={`size-2 shrink-0 rounded-full ${s.available ? 'bg-emerald-500' : 'bg-neutral-300'}`}
              />
              <span className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
                <button
                  title="刷新探测"
                  onClick={(e) => {
                    e.stopPropagation()
                    refreshMut.mutate(s.id)
                  }}
                  className="rounded p-0.5 hover:bg-neutral-200"
                >
                  <RefreshCw className="size-3.5" />
                </button>
                <button
                  title="移除"
                  onClick={(e) => {
                    e.stopPropagation()
                    remove(s)
                  }}
                  className="rounded p-0.5 text-red-500 hover:bg-red-50"
                >
                  <Trash2 className="size-3.5" />
                </button>
              </span>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <aside className="flex w-60 shrink-0 flex-col rounded-xl border border-neutral-200 bg-white">
      <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2.5">
        <p className="text-sm font-medium text-neutral-900">存储卷</p>
        <button
          onClick={() => setAdding((v) => !v)}
          title="添加存储卷"
          className="rounded-lg p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
        >
          {adding ? <X className="size-4" /> : <Plus className="size-4" />}
        </button>
      </div>
      <div className="flex-1 space-y-4 overflow-y-auto p-2">
        {adding && (
          <form
            onSubmit={(e) => {
              e.preventDefault()
              setFormError('')
              createMut.mutate({
                name: form.name,
                type: form.type,
                root_path: form.root_path,
                device_id: form.type === 'external' ? form.device_id : undefined,
              })
            }}
            className="space-y-2 rounded-lg border border-indigo-100 bg-indigo-50/40 p-2 text-sm"
          >
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="名称（如：电影）"
              required
              className="w-full rounded-md border border-neutral-200 bg-white px-2 py-1.5 outline-none focus:border-indigo-400"
            />
            <select
              value={form.type}
              onChange={(e) => setForm({ ...form, type: e.target.value as StorageType })}
              className="w-full rounded-md border border-neutral-200 bg-white px-2 py-1.5 outline-none focus:border-indigo-400"
            >
              <option value="internal">内置</option>
              <option value="external">外接</option>
              <option value="network">网络</option>
            </select>
            <input
              value={form.root_path}
              onChange={(e) => setForm({ ...form, root_path: e.target.value })}
              placeholder="根路径，如 D:\Videos"
              required
              className="w-full rounded-md border border-neutral-200 bg-white px-2 py-1.5 outline-none focus:border-indigo-400"
            />
            {form.type === 'external' && (
              <input
                value={form.device_id}
                onChange={(e) => setForm({ ...form, device_id: e.target.value })}
                placeholder="卷序列号（外接必填）"
                required
                className="w-full rounded-md border border-neutral-200 bg-white px-2 py-1.5 outline-none focus:border-indigo-400"
              />
            )}
            {formError && <p className="text-xs text-red-500">{formError}</p>}
            <button
              type="submit"
              disabled={createMut.isPending}
              className="flex w-full items-center justify-center gap-1.5 rounded-md bg-indigo-600 px-2 py-1.5 font-medium text-white hover:bg-indigo-500 disabled:opacity-60"
            >
              {createMut.isPending && <Loader2 className="size-3.5 animate-spin" />}
              添加
            </button>
          </form>
        )}
        {isLoading ? (
          <div className="flex justify-center py-6 text-neutral-400">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : (
          <>
            {renderGroup(storages.filter((s) => s.available), '在线')}
            {renderGroup(storages.filter((s) => !s.available), '离线')}
            {storages.length === 0 && (
              <p className="px-2 py-4 text-center text-sm text-neutral-400">暂无存储卷，点击右上角添加</p>
            )}
          </>
        )}
      </div>
    </aside>
  )
}
