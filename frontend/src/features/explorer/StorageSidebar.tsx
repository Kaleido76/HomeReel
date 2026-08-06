import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronsLeft, ChevronsRight, HardDrive, Loader2, Network, Plus, RefreshCw, Trash2, Usb, X } from 'lucide-react'
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

// StorageSidebar is collapsible: on wide screens it defaults to a slim icon rail
// (volumes change rarely, so they need not occupy a large area) and expands to
// the full management panel on click; the toggle persists per session. The rail
// itself is a horizontal chip strip on narrow screens.
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
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      {/* Narrow: horizontal chip strip */}
      <div className="flex items-center gap-2 overflow-x-auto pb-1 lg:hidden">
        {storages.map((s) => {
          const Icon = typeIcons[s.type]
          return (
            <button
              key={s.id}
              onClick={() => onSelect(s.id)}
              className={`flex shrink-0 items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors ${
                s.id === selectedId
                  ? 'border-blue-600 bg-blue-50 text-blue-700'
                  : 'border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50'
              }`}
            >
              <Icon className="size-4 shrink-0" />
              <span className="max-w-32 truncate">{s.name}</span>
              <span
                title={s.available ? `${typeLabels[s.type]} · 在线` : `${typeLabels[s.type]} · 离线`}
                className={`size-2 shrink-0 rounded-full ${s.available ? 'bg-emerald-500' : 'bg-neutral-300'}`}
              />
            </button>
          )
        })}
        {storages.length === 0 && (
          <p className="shrink-0 text-sm text-neutral-400">暂无存储卷，请先添加</p>
        )}
      </div>

      {/* Wide: collapsible rail / panel */}
      <div className="hidden h-full lg:block">
        {expanded ? (
          <StoragePanel
            storages={storages}
            isLoading={isLoading}
            selectedId={selectedId}
            onSelect={onSelect}
            onCollapse={() => setExpanded(false)}
          />
        ) : (
          <aside className="flex h-full w-14 shrink-0 flex-col items-center rounded-xl border border-neutral-200 bg-white py-2">
            <button
              onClick={() => setExpanded(true)}
              title="展开存储卷列表"
              className="mb-1 rounded-lg p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
            >
              <ChevronsRight className="size-4" />
            </button>
            {isLoading ? (
              <div className="flex justify-center py-6 text-neutral-400">
                <Loader2 className="size-4 animate-spin" />
              </div>
            ) : (
              <div className="flex flex-1 flex-col items-center gap-2 overflow-y-auto">
                {storages.map((s) => {
                  const Icon = typeIcons[s.type]
                  return (
                    <button
                      key={s.id}
                      onClick={() => onSelect(s.id)}
                      title={`${s.name} · ${s.available ? '在线' : '离线'}${s.busy ? ' · 扫描中' : ''}`}
                      className={`relative rounded-lg p-2 transition-colors ${
                        s.id === selectedId ? 'bg-blue-50 text-blue-600' : 'text-neutral-500 hover:bg-neutral-100'
                      }`}
                    >
                      <Icon className="size-5" />
                      <span
                        className={`absolute right-1 top-1 size-2 rounded-full ${
                          s.available ? 'bg-emerald-500' : 'bg-neutral-300'
                        }`}
                      />
                      {s.busy && (
                        <span className="absolute -left-0.5 -top-0.5 flex h-3.5 w-3.5 items-center justify-center">
                          <span className="absolute inline-flex h-3 w-3 animate-ping rounded-full bg-blue-400 opacity-75" />
                          <span className="relative inline-flex h-2 w-2 rounded-full bg-blue-600" />
                        </span>
                      )}
                    </button>
                  )
                })}
              </div>
            )}
          </aside>
        )}
      </div>
    </>
  )
}

// StoragePanel is the expanded management sidebar (wide screens only): grouped
// volume list with refresh/remove actions and the create-volume form.
function StoragePanel({
  storages,
  isLoading,
  selectedId,
  onSelect,
  onCollapse,
}: {
  storages: Storage[]
  isLoading: boolean
  selectedId: string
  onSelect: (id: string) => void
  onCollapse: () => void
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
                s.id === selectedId ? 'bg-blue-50 text-blue-700' : 'text-neutral-700 hover:bg-neutral-100'
              }`}
            >
              <Icon className="size-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{s.name}</span>
              {s.busy && (
                <span className="shrink-0 rounded bg-amber-50 px-1.5 py-0.5 text-[10px] text-amber-600">扫描中</span>
              )}
              <span
                title={s.available ? `${typeLabels[s.type]} · 在线` : `${typeLabels[s.type]} · 离线`}
                className={`size-2 shrink-0 rounded-full ${s.available ? 'bg-emerald-500' : 'bg-neutral-300'}`}
              />
              <span className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
                <button
                  title="刷新探测"
                  disabled={s.busy}
                  onClick={(e) => {
                    e.stopPropagation()
                    refreshMut.mutate(s.id)
                  }}
                  className="rounded p-0.5 hover:bg-neutral-200 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  {s.busy ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
                </button>
                <button
                  title={s.busy ? '扫描完成后才能移除' : '移除'}
                  disabled={s.busy}
                  onClick={(e) => {
                    e.stopPropagation()
                    remove(s)
                  }}
                  className="rounded p-0.5 text-red-500 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
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
    <aside className="flex h-full w-60 shrink-0 flex-col rounded-xl border border-neutral-200 bg-white">
      <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2.5">
        <p className="text-sm font-medium text-neutral-900">存储卷</p>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setAdding((v) => !v)}
            title="添加存储卷"
            className="rounded-lg p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
          >
            {adding ? <X className="size-4" /> : <Plus className="size-4" />}
          </button>
          <button
            onClick={onCollapse}
            title="收起"
            className="rounded-lg p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
          >
            <ChevronsLeft className="size-4" />
          </button>
        </div>
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
            className="space-y-2 rounded-lg border border-neutral-200 bg-neutral-50 p-2 text-sm"
          >
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="名称（如：电影）"
              required
              className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 outline-none focus:border-blue-600"
            />
            <select
              value={form.type}
              onChange={(e) => setForm({ ...form, type: e.target.value as StorageType })}
              className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 outline-none focus:border-blue-600"
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
              className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 outline-none focus:border-blue-600"
            />
            {form.type === 'external' && (
              <input
                value={form.device_id}
                onChange={(e) => setForm({ ...form, device_id: e.target.value })}
                placeholder="卷序列号（外接必填）"
                required
                className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 outline-none focus:border-blue-600"
              />
            )}
            {formError && <p className="text-xs text-red-500">{formError}</p>}
            <button
              type="submit"
              disabled={createMut.isPending}
              className="flex w-full items-center justify-center gap-1.5 rounded-md bg-blue-600 px-2 py-1.5 font-medium text-white hover:bg-blue-700 disabled:opacity-60"
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
