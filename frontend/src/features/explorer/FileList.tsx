import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowUp,
  Check,
  Download,
  File,
  FileVideo,
  Folder,
  FolderPlus,
  Loader2,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { ApiError } from '../../api/client'
import {
  fetchFsList,
  fsDelete,
  fsDownloadUrl,
  fsMkdir,
  fsMove,
  fsRename,
} from '../../api/storages'
import { uploadChunked } from './upload'

const btnCls =
  'flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:cursor-not-allowed disabled:opacity-50'

export function FileList({
  storageId,
  path,
  onOpen,
}: {
  storageId: string
  path: string
  onOpen: (path: string) => void
}) {
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const list = useQuery({
    queryKey: ['fs-list', storageId, path],
    queryFn: () => fetchFsList(storageId, path),
  })

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [renaming, setRenaming] = useState('')
  const [renameValue, setRenameValue] = useState('')
  const [creating, setCreating] = useState(false)
  const [dirName, setDirName] = useState('')
  const [moving, setMoving] = useState(false)
  const [moveDest, setMoveDest] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [uploading, setUploading] = useState(false)
  const [uploadName, setUploadName] = useState('')
  const [uploadProgress, setUploadProgress] = useState(0)

  const readonly = list.data?.storage.readonly ?? false
  const entries = list.data?.entries ?? []
  const parent = path.split('/').slice(0, -1).join('/')

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ['fs-list', storageId, path] })
  }

  function toggle(path: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })
  }

  async function run(fn: () => Promise<unknown>, onDone?: () => void) {
    setBusy(true)
    setError('')
    try {
      await fn()
      invalidate()
      onDone?.()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  function download(rel: string) {
    const a = document.createElement('a')
    a.href = fsDownloadUrl(storageId, rel)
    a.download = rel.split('/').pop() ?? 'download'
    a.click()
  }

  async function onFiles(files: FileList) {
    const targets = Array.from(files)
    if (targets.length === 0) return
    setUploading(true)
    setError('')
    try {
      for (const file of targets) {
        setUploadName(file.name)
        setUploadProgress(0)
        await uploadChunked(storageId, path, file, (ratio) => setUploadProgress(ratio))
      }
      invalidate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '上传失败')
    } finally {
      setUploading(false)
      setUploadName('')
      setUploadProgress(0)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  if (list.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-neutral-400">
        <Loader2 className="size-5 animate-spin" />
      </div>
    )
  }

  if (list.isError) {
    const offline = list.error instanceof ApiError && list.error.status === 409
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-neutral-400">
        <p>{offline ? '存储卷当前离线，请检查设备连接' : list.error.message}</p>
        <button
          onClick={() => list.refetch()}
          className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50"
        >
          <RefreshCw className="size-4" /> 重试
        </button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-neutral-100 px-3 py-2">
        {!readonly && (
          <>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => e.target.files && onFiles(e.target.files)}
            />
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={busy || uploading}
              className={btnCls}
            >
              <Upload className="size-4" /> <span className="hidden sm:inline">上传</span>
            </button>
            <button
              onClick={() => {
                setCreating(true)
                setDirName('')
              }}
              disabled={busy}
              className={btnCls}
            >
              <FolderPlus className="size-4" /> <span className="hidden sm:inline">新建文件夹</span>
            </button>
          </>
        )}
        {!readonly && selected.size > 0 && (
          <>
            <button
              onClick={() => {
                setMoving(true)
                setMoveDest('')
              }}
              disabled={busy}
              className={btnCls}
            >
              <Folder className="size-4" /> <span className="hidden sm:inline">移动到</span>
            </button>
            <button
              onClick={() => {
                if (window.confirm(`删除选中的 ${selected.size} 项？此操作不可恢复。`)) {
                  run(() => fsDelete(storageId, Array.from(selected)), () => setSelected(new Set()))
                }
              }}
              disabled={busy}
              className="flex items-center gap-1.5 rounded-lg border border-red-200 px-2.5 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
            >
              <Trash2 className="size-4" /> <span className="hidden sm:inline">删除</span>
            </button>
          </>
        )}
        <span className="ml-auto text-xs text-neutral-400">
          {entries.length} 项{selected.size > 0 && ` · 已选 ${selected.size} 项`}
        </span>
      </div>

      {creating && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            run(() => fsMkdir(storageId, path, dirName), () => {
              setCreating(false)
              setDirName('')
            })
          }}
          className="flex items-center gap-2 border-b border-neutral-100 bg-neutral-50 px-3 py-2"
        >
          <FolderPlus className="size-4 text-neutral-400" />
          <input
            value={dirName}
            onChange={(e) => setDirName(e.target.value)}
            placeholder="文件夹名称"
            autoFocus
            className="rounded-md border border-neutral-300 bg-white px-2 py-1 text-sm outline-none focus:border-blue-600"
          />
          <button type="submit" disabled={busy || !dirName.trim()} className={btnCls}>
            <Check className="size-4" /> 创建
          </button>
          <button type="button" onClick={() => setCreating(false)} className={btnCls}>
            取消
          </button>
        </form>
      )}

      {moving && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            run(
              () => fsMove(storageId, Array.from(selected), moveDest),
              () => {
                setMoving(false)
                setSelected(new Set())
              },
            )
          }}
          className="flex items-center gap-2 border-b border-neutral-100 bg-neutral-50 px-3 py-2"
        >
          <Folder className="size-4 text-neutral-400" />
          <input
            value={moveDest}
            onChange={(e) => setMoveDest(e.target.value)}
            placeholder="目标目录（相对存储卷根，需已存在）"
            autoFocus
            className="flex-1 rounded-md border border-neutral-300 bg-white px-2 py-1 text-sm outline-none focus:border-blue-600"
          />
          <button type="submit" disabled={busy} className={btnCls}>
            移动
          </button>
          <button type="button" onClick={() => setMoving(false)} className={btnCls}>
            取消
          </button>
        </form>
      )}

      {uploading && (
        <div className="flex items-center gap-3 border-b border-neutral-100 bg-neutral-50 px-3 py-2 text-sm">
          <Loader2 className="size-4 animate-spin text-blue-600" />
          <span className="truncate text-neutral-700">{uploadName}</span>
          <div className="h-1.5 flex-1 overflow-hidden rounded-sm bg-neutral-200">
            <div
              className="h-full rounded-sm bg-blue-600 transition-all"
              style={{ width: `${Math.round(uploadProgress * 100)}%` }}
            />
          </div>
          <span className="text-neutral-500">{Math.round(uploadProgress * 100)}%</span>
        </div>
      )}

      {error && <p className="border-b border-neutral-100 bg-red-50 px-3 py-2 text-sm text-red-600">{error}</p>}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-neutral-50 text-left text-neutral-500">
            <tr className="border-b border-neutral-100">
              {!readonly && <th className="w-8 px-2 py-2" />}
              <th className="px-4 py-2 font-medium">名称</th>
              <th className="hidden w-28 px-4 py-2 font-medium md:table-cell">大小</th>
              <th className="hidden w-44 px-4 py-2 font-medium lg:table-cell">修改时间</th>
              <th className="w-28 px-4 py-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {path && (
              <tr onClick={() => onOpen(parent)} className="cursor-pointer hover:bg-neutral-50">
                <td colSpan={readonly ? 4 : 5} className="px-4 py-2">
                  <span className="flex items-center gap-2 text-neutral-500">
                    <ArrowUp className="size-4" /> ..
                  </span>
                </td>
              </tr>
            )}
            {entries.map((e) => (
              <tr
                key={e.path}
                onClick={() => e.is_dir && onOpen(e.path)}
                className={`group ${e.is_dir ? 'cursor-pointer hover:bg-neutral-50' : ''} border-b border-neutral-50`}
              >
                {!readonly && (
                  <td className="px-2 py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(e.path)}
                      onChange={() => toggle(e.path)}
                      onClick={(ev) => ev.stopPropagation()}
                      className="accent-blue-600"
                    />
                  </td>
                )}
                <td className="px-4 py-2">
                  {renaming === e.path ? (
                    <form
                      onSubmit={(ev) => {
                        ev.preventDefault()
                        run(() => fsRename(storageId, e.path, renameValue), () => setRenaming(''))
                      }}
                      className="flex items-center gap-1"
                    >
                      <input
                        value={renameValue}
                        onChange={(ev) => setRenameValue(ev.target.value)}
                        autoFocus
                        className="rounded-md border border-neutral-300 px-1.5 py-0.5 text-sm outline-none focus:border-blue-600"
                      />
                      <button type="submit" disabled={busy || !renameValue.trim()} title="确认">
                        <Check className="size-4 text-emerald-600" />
                      </button>
                      <button type="button" onClick={() => setRenaming('')} title="取消">
                        <X className="size-4 text-neutral-400" />
                      </button>
                    </form>
                  ) : (
                    <span className="flex items-center gap-2 text-neutral-800">
                      {e.is_dir ? (
                        <Folder className="size-4 shrink-0 text-neutral-400" />
                      ) : e.is_video ? (
                        <FileVideo className="size-4 shrink-0 text-blue-600" />
                      ) : (
                        <File className="size-4 shrink-0 text-neutral-400" />
                      )}
                      {e.name}
                    </span>
                  )}
                </td>
                <td className="hidden px-4 py-2 text-neutral-500 md:table-cell">
                  {e.is_dir ? '—' : formatBytes(e.size)}
                </td>
                <td className="hidden px-4 py-2 text-neutral-500 lg:table-cell">
                  {e.mtime ? formatTime(e.mtime) : ''}
                </td>
                <td className="px-2 py-2 text-right">
                  {renaming !== e.path && (
                    <span className="flex items-center justify-end gap-1 opacity-100 sm:opacity-0 sm:group-hover:opacity-100">
                      {!e.is_dir && (
                        <button
                          title="下载"
                          onClick={(ev) => {
                            ev.stopPropagation()
                            download(e.path)
                          }}
                          className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
                        >
                          <Download className="size-4" />
                        </button>
                      )}
                      {!readonly && (
                        <>
                          <button
                            title="重命名"
                            onClick={(ev) => {
                              ev.stopPropagation()
                              setRenaming(e.path)
                              setRenameValue(e.name)
                            }}
                            className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
                          >
                            <Pencil className="size-4" />
                          </button>
                          <button
                            title="删除"
                            onClick={(ev) => {
                              ev.stopPropagation()
                              if (window.confirm(`删除「${e.name}」？此操作不可恢复。`)) {
                                run(() => fsDelete(storageId, [e.path]))
                              }
                            }}
                            className="rounded p-1 text-red-500 hover:bg-red-50"
                          >
                            <Trash2 className="size-4" />
                          </button>
                        </>
                      )}
                    </span>
                  )}
                </td>
              </tr>
            ))}
            {entries.length === 0 && (
              <tr>
                <td colSpan={readonly ? 4 : 5} className="px-4 py-10 text-center text-neutral-400">
                  空目录
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`
}

function formatTime(sec: number): string {
  return new Date(sec * 1000).toLocaleString()
}
