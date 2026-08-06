import { useRef, useState } from 'react'
import { useQueries, useQueryClient } from '@tanstack/react-query'
import {
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

// ColumnBrowser is the wide-screen Finder-style multi-column view: every
// directory level from the storage root down to the current path renders as one
// column with its own listing, so drilling into a folder opens the next column
// to the right (and clicking a column header jumps back to that level). Only
// the deepest column is interactive (selection, move/delete, rename) and carries
// the action toolbar; ancestor columns are navigation-only.
export function ColumnBrowser({
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

  const segments = path ? path.split('/') : []
  const levels: string[] = ['', ...segments.map((_, i) => segments.slice(0, i + 1).join('/'))]
  const current = levels[levels.length - 1]

  const columns = useQueries({
    queries: levels.map((lv) => ({
      queryKey: ['fs-list', storageId, lv],
      queryFn: () => fetchFsList(storageId, lv),
      // 轮询以感知扫描锁定（同 FileList）
      refetchInterval: 5000,
    })),
  })

  const list = columns[columns.length - 1]
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
  const scanning = list.data?.storage.busy ?? false
  const entries = list.data?.entries ?? []

  function invalidate() {
    for (const lv of levels) {
      queryClient.invalidateQueries({ queryKey: ['fs-list', storageId, lv] })
    }
  }

  function toggle(p: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(p)) {
        next.delete(p)
      } else {
        next.add(p)
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
        await uploadChunked(storageId, current, file, (ratio) => setUploadProgress(ratio))
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

  return (
    <div className="flex h-full flex-col">
      {scanning && (
        <div className="flex items-center gap-2 border-b border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
          <Loader2 className="size-4 animate-spin" />
          <span>该存储卷正在扫描，文件操作暂时锁定</span>
        </div>
      )}
      {/* Action toolbar for the current (deepest) directory */}
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
            <button onClick={() => fileInputRef.current?.click()} disabled={busy || uploading || scanning} className={btnCls}>
              <Upload className="size-4" /> 上传
            </button>
            <button
              onClick={() => {
                setCreating(true)
                setDirName('')
              }}
              disabled={busy || scanning}
              className={btnCls}
            >
              <FolderPlus className="size-4" /> 新建文件夹
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
              disabled={busy || scanning}
              className={btnCls}
            >
              <Folder className="size-4" /> 移动到
            </button>
            <button
              onClick={() => {
                if (window.confirm(`删除选中的 ${selected.size} 项？此操作不可恢复。`)) {
                  run(() => fsDelete(storageId, Array.from(selected)), () => setSelected(new Set()))
                }
              }}
              disabled={busy || scanning}
              className="flex items-center gap-1.5 rounded-lg border border-red-200 px-2.5 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
            >
              <Trash2 className="size-4" /> 删除
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
            run(() => fsMkdir(storageId, current, dirName), () => {
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
          <button type="submit" disabled={busy || scanning || !dirName.trim()} className={btnCls}>
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
          <button type="submit" disabled={busy || scanning} className={btnCls}>
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

      {/* The column strip: one column per directory level */}
      <div className="flex min-h-0 flex-1 divide-x divide-neutral-100 overflow-x-auto">
        {levels.map((lv, i) => {
          const isLast = i === levels.length - 1
          const col = columns[i]
          const colEntries = col.data?.entries ?? []
          return (
            <div key={lv || 'root'} className="flex w-56 shrink-0 flex-col sm:w-64">
              <button
                onClick={() => onOpen(lv)}
                className={`flex shrink-0 items-center gap-1.5 border-b border-neutral-100 px-3 py-2 text-left text-xs font-medium ${
                  isLast ? 'bg-white text-neutral-900' : 'bg-neutral-50 text-neutral-500 hover:text-neutral-800'
                }`}
                title={lv === '' ? '存储卷根目录' : lv}
              >
                <Folder className="size-3.5 shrink-0 text-neutral-400" />
                <span className="min-w-0 flex-1 truncate">{lv === '' ? '根目录' : lv.split('/').pop()}</span>
              </button>
              <div className="min-h-0 flex-1 overflow-y-auto bg-white">
                {col.isLoading ? (
                  <div className="flex justify-center py-8 text-neutral-400">
                    <Loader2 className="size-4 animate-spin" />
                  </div>
                ) : col.isError ? (
                  <div className="flex flex-col items-center gap-2 px-3 py-6 text-center text-sm text-neutral-400">
                    <p>{col.error?.message}</p>
                    <button
                      onClick={() => col.refetch()}
                      className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2.5 py-1 text-xs text-neutral-600 hover:bg-neutral-50"
                    >
                      <RefreshCw className="size-3" /> 重试
                    </button>
                  </div>
                ) : (
                  <ul className="py-1">
                    {colEntries.map((e) => (
                      <li key={e.path}>
                        {e.is_dir ? (
                          <button
                            onClick={() => onOpen(e.path)}
                            className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-neutral-700 hover:bg-neutral-50"
                          >
                            <Folder className="size-4 shrink-0 text-neutral-400" />
                            <span className="min-w-0 flex-1 truncate">{e.name}</span>
                          </button>
                        ) : (
                          <div className="group flex items-center gap-2 px-3 py-1.5 text-sm text-neutral-800">
                            {isLast && !readonly && (
                              <input
                                type="checkbox"
                                checked={selected.has(e.path)}
                                onChange={() => toggle(e.path)}
                                onClick={(ev) => ev.stopPropagation()}
                                disabled={scanning}
                                className="accent-blue-600"
                              />
                            )}
                            {e.is_video ? (
                              <FileVideo className="size-4 shrink-0 text-blue-600" />
                            ) : (
                              <File className="size-4 shrink-0 text-neutral-400" />
                            )}
                            <span className="min-w-0 flex-1 truncate" title={e.name}>
                              {e.name}
                            </span>
                            {isLast && renaming === e.path ? (
                              <RenameForm
                                busy={busy || scanning}
                                value={renameValue}
                                onChange={setRenameValue}
                                onCancel={() => setRenaming('')}
                                onSubmit={() =>
                                  run(() => fsRename(storageId, e.path, renameValue), () => setRenaming(''))
                                }
                              />
                            ) : isLast ? (
                              <span className="flex shrink-0 items-center gap-0.5 opacity-0 group-hover:opacity-100">
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
                                {!readonly && (
                                  <>
                                    <button
                                      title="重命名"
                                      disabled={scanning}
                                      onClick={(ev) => {
                                        ev.stopPropagation()
                                        setRenaming(e.path)
                                        setRenameValue(e.name)
                                      }}
                                      className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 disabled:opacity-40"
                                    >
                                      <Pencil className="size-4" />
                                    </button>
                                    <button
                                      title="删除"
                                      disabled={scanning}
                                      onClick={(ev) => {
                                        ev.stopPropagation()
                                        if (window.confirm(`删除「${e.name}」？此操作不可恢复。`)) {
                                          run(() => fsDelete(storageId, [e.path]))
                                        }
                                      }}
                                      className="rounded p-1 text-red-500 hover:bg-red-50 disabled:opacity-40"
                                    >
                                      <Trash2 className="size-4" />
                                    </button>
                                  </>
                                )}
                              </span>
                            ) : null}
                          </div>
                        )}
                      </li>
                    ))}
                    {colEntries.length === 0 && (
                      <li className="px-3 py-8 text-center text-sm text-neutral-400">空目录</li>
                    )}
                  </ul>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function RenameForm({
  busy,
  value,
  onChange,
  onCancel,
  onSubmit,
}: {
  busy: boolean
  value: string
  onChange: (v: string) => void
  onCancel: () => void
  onSubmit: () => void
}) {
  return (
    <form
      onSubmit={(ev) => {
        ev.preventDefault()
        onSubmit()
      }}
      className="flex shrink-0 items-center gap-1"
    >
      <input
        value={value}
        onChange={(ev) => onChange(ev.target.value)}
        autoFocus
        className="w-24 rounded-md border border-neutral-300 px-1.5 py-0.5 text-xs outline-none focus:border-blue-600"
      />
      <button type="submit" disabled={busy || !value.trim()} title="确认">
        <Check className="size-4 text-emerald-600" />
      </button>
      <button type="button" onClick={onCancel} title="取消">
        <X className="size-4 text-neutral-400" />
      </button>
    </form>
  )
}
