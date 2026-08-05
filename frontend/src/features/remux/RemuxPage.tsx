import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, FolderCog, Loader2, RefreshCw } from 'lucide-react'
import { fetchStorages } from '../../api/storages'
import { fetchRemuxStatus, requestFolderRemux, requestRemux } from '../../api/remux'

// RemuxPage is the segmented-MP4 remux manager: hls.js-downloaded files (many
// mdat boxes) make desktop Chrome download the whole file before playing. A
// remux re-wraps such a file into a standard faststart MP4 (stream copy, no
// re-encode) after which direct playback is fast and fully seekable. Remuxing
// is user-driven — nothing runs automatically at scan time.
export function RemuxPage() {
  const queryClient = useQueryClient()
  const [requested, setRequested] = useState<Set<string>>(new Set())

  const status = useQuery({
    queryKey: ['remux', 'status'],
    queryFn: fetchRemuxStatus,
    refetchInterval: 5000,
  })
  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })
  const storageById = useMemo(() => {
    const map = new Map<string, { name: string }>()
    for (const s of storages.data?.storages ?? []) map.set(s.id, s)
    return map
  }, [storages.data])

  const [folderStorage, setFolderStorage] = useState('')
  const [folderPath, setFolderPath] = useState('')
  const [folderMsg, setFolderMsg] = useState('')

  const items = status.data?.items ?? []

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['remux', 'status'] })

  const remuxOne = useMutation({
    mutationFn: requestRemux,
    onSuccess: (_, id) => {
      setRequested((prev) => new Set(prev).add(id))
      void invalidate()
    },
  })

  const remuxFolder = useMutation({
    mutationFn: () => requestFolderRemux(folderStorage, folderPath.trim()),
    onSuccess: (res) => {
      setFolderMsg(res.accepted > 0 ? `已提交 ${res.accepted} 个视频的重封请求` : '该文件夹下没有分段视频')
      setFolderPath('')
      void invalidate()
    },
  })

  const pendingRemux = items.some((it) => !it.remuxed)

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-neutral-900">重封管理</h1>
          <p className="mt-1 max-w-2xl text-sm text-neutral-500">
            部分从网页下载的视频（hls.js 拼接）在电脑浏览器里会整文件下载后才能播放。重封可将其转换为标准
            faststart MP4（仅重封装、不重编码），之后秒开且可拖动。
          </p>
        </div>
        <button
          onClick={() => {
            const ids = items.filter((it) => !it.remuxed).map((it) => it.video_id)
            ids.forEach((id) => remuxOne.mutate(id))
          }}
          disabled={!pendingRemux || remuxOne.isPending}
          className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          {remuxOne.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
          重封全部未重封
        </button>
      </div>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          setFolderMsg('')
          remuxFolder.mutate()
        }}
        className="flex flex-wrap items-end gap-2 rounded-lg border border-neutral-200 bg-white p-3"
      >
        <div className="min-w-40">
          <label className="mb-1 block text-xs text-neutral-500">存储卷</label>
          <select
            value={folderStorage}
            onChange={(e) => setFolderStorage(e.target.value)}
            required
            className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-blue-600"
          >
            <option value="">选择存储卷…</option>
            {(storages.data?.storages ?? []).map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </div>
        <div className="min-w-52 flex-1">
          <label className="mb-1 block text-xs text-neutral-500">文件夹路径（相对存储卷根目录，留空表示根目录）</label>
          <input
            value={folderPath}
            onChange={(e) => setFolderPath(e.target.value)}
            placeholder="如 TV/黑袍纠察队 第三季"
            className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-blue-600"
          />
        </div>
        <button
          type="submit"
          disabled={!folderStorage || remuxFolder.isPending}
          className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          {remuxFolder.isPending ? <Loader2 className="size-4 animate-spin" /> : <FolderCog className="size-4" />}
          重封此文件夹
        </button>
        {folderMsg && <p className="basis-full text-sm text-neutral-500">{folderMsg}</p>}
      </form>

      {status.isLoading ? (
        <div className="flex h-40 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      ) : status.isError ? (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
          {status.error.message}
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-lg border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          暂无分段视频。检测到 hls.js 拼接的 MP4 时会出现在这里。
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
          <ul className="divide-y divide-neutral-100">
            {items.map((it) => {
              const done = it.remuxed
              const submitted = requested.has(it.video_id) && !done
              const storageName = storageById.get(it.storage_id)?.name ?? it.storage_id
              return (
                <li key={it.video_id} className="flex items-center gap-3 px-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-neutral-800" title={it.title}>
                      {it.title}
                    </p>
                    <p className="truncate text-xs text-neutral-400" title={it.relative_path}>
                      {storageName} / {it.relative_path}
                    </p>
                  </div>
                  <span
                    className={`shrink-0 rounded px-2 py-0.5 text-xs ${
                      done ? 'bg-emerald-50 text-emerald-600' : 'bg-neutral-100 text-neutral-500'
                    }`}
                  >
                    {done ? '已重封' : submitted ? '已提交' : '未重封'}
                  </span>
                  <button
                    onClick={() => remuxOne.mutate(it.video_id)}
                    disabled={done || submitted || remuxOne.isPending}
                    className="flex shrink-0 items-center gap-1 rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
                  >
                    {done ? <CheckCircle2 className="size-4 text-emerald-600" /> : <RefreshCw className="size-3.5" />}
                    {done ? '完成' : submitted ? '排队中' : '重封'}
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}
