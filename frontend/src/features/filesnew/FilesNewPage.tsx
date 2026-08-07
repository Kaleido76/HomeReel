import { useState } from 'react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../../api/client'
import {
  addPin,
  fetchDisks,
  fetchFs2List,
  fetchPins,
  fs2Copy,
  fs2Delete,
  fs2Move,
  fs2Rename,
  removePin,
} from '../../api/fsbrowse'
import { DriveRail } from './DriveRail'
import { Toolbar, type ClipMode, type Clipboard, type SortState } from './Toolbar'
import { FileListView } from './FileListView'
import { ConfirmDelete } from './ConfirmDelete'
import { isMediaName } from './fileType'
import { parentPath } from './path'

interface FilesNewSearch {
  path: string
}

// FilesNewPage is the generic machine-wide file browser (文件（新） tab): it
// lists directories by absolute path on demand (nothing is indexed) and offers
// Windows-Explorer-style clipboard operations (cut/copy/paste/rename/delete).
// The current path lives in the URL so refresh/deep-link restores the folder.
export function FilesNewPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { path } = (location.search ?? {}) as FilesNewSearch

  const [sort, setSort] = useState<SortState>({ key: 'name', dir: 'asc' })
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [clipboard, setClipboard] = useState<Clipboard | null>(null)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string[] | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [mediaOnly, setMediaOnly] = useState(false)

  const disks = useQuery({ queryKey: ['fs2-disks'], queryFn: fetchDisks, staleTime: 60_000 })
  const pins = useQuery({ queryKey: ['fs2-pins'], queryFn: fetchPins })
  const list = useQuery({
    queryKey: ['fs2-list', path],
    queryFn: () => fetchFs2List(path),
    enabled: !!path,
    // 轮询以感知后台复制/移动任务完成后目录内容的变化（同旧文件页的 busy 轮询）
    refetchInterval: 5000,
  })

  const entries = list.data?.entries ?? []
  // 多媒体视图：目录始终保留以便继续导航，文件仅显示视频/音乐。
  const visibleEntries = mediaOnly ? entries.filter((e) => e.is_dir || isMediaName(e.name)) : entries

  function go(p: string) {
    setSelected(new Set())
    setRenaming(null)
    navigate({ to: '/filesnew', search: { path: p } })
  }

  function goUp() {
    const parent = parentPath(path)
    if (parent) go(parent)
  }

  function flash(message: string) {
    setError('')
    setNotice(message)
    window.setTimeout(() => setNotice(''), 4000)
  }

  function invalidate() {
    if (path) queryClient.invalidateQueries({ queryKey: ['fs2-list', path] })
    queryClient.invalidateQueries({ queryKey: ['fs2-pins'] })
  }

  function toggleSelect(p: string) {
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

  function selectSingle(p: string) {
    setSelected(new Set([p]))
  }

  function setClipboardMode(mode: ClipMode) {
    if (selected.size === 0) return
    setClipboard({ mode, items: Array.from(selected) })
  }

  async function paste() {
    if (!clipboard || !path) return
    setError('')
    try {
      if (clipboard.mode === 'copy') {
        await fs2Copy(clipboard.items, path)
      } else {
        await fs2Move(clipboard.items, path)
      }
      const verb = clipboard.mode === 'copy' ? '复制' : '移动'
      flash(`${verb}已作为后台任务开始，可在顶部任务面板查看进度`)
      if (clipboard.mode === 'cut') setClipboard(null)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '操作失败')
    }
  }

  async function commitRename(p: string, newName: string) {
    setError('')
    try {
      await fs2Rename(p, newName)
      setRenaming(null)
      invalidate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '重命名失败')
    }
  }

  async function confirmDelete() {
    if (!deleting) return
    setError('')
    try {
      const res = await fs2Delete(deleting)
      if (res.errors && res.errors.length > 0) {
        setError(res.errors.map((e) => `${e.path}: ${e.message}`).join('；'))
      } else {
        flash(`已删除 ${deleting.length} 项`)
      }
      setDeleting(null)
      setSelected(new Set())
      invalidate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '删除失败')
    }
  }

  async function togglePin() {
    if (!path) return
    setError('')
    try {
      if (pinned) {
        await removePin(path)
        flash('已取消固定')
      } else {
        await addPin(path)
        flash('已固定当前目录')
      }
      // 立即刷新 pin 列表，避免左侧面板需刷新页面才更新
      await queryClient.invalidateQueries({ queryKey: ['fs2-pins'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : pinned ? '取消固定失败' : '固定失败')
    }
  }

  const pinned = pins.data?.pins.includes(path) ?? false
  const selectedCount = selected.size

  return (
    <div className="flex h-full min-h-0">
      <DriveRail
        disks={disks.data?.disks ?? []}
        pins={pins.data?.pins ?? []}
        currentPath={path}
        onNavigate={go}
      />

      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Toolbar
          path={path}
          canGoUp={parentPath(path) !== null}
          entryCount={visibleEntries.length}
          selectedCount={selectedCount}
          clipboard={clipboard}
          sort={sort}
          onSortChange={setSort}
          onNavigate={go}
          onGoUp={goUp}
          onCut={() => setClipboardMode('cut')}
          onCopy={() => setClipboardMode('copy')}
          onPaste={paste}
          onRename={() => {
            if (selectedCount === 1) setRenaming(Array.from(selected)[0])
          }}
          onDelete={() => {
            if (selectedCount > 0) setDeleting(Array.from(selected))
          }}
          onPin={togglePin}
          pinned={pinned}
          mediaOnly={mediaOnly}
          onToggleMedia={() => setMediaOnly((v) => !v)}
          notice={notice}
        />

        {error && <p className="border-b border-neutral-100 bg-red-50 px-3 py-2 text-sm text-red-600">{error}</p>}

        <FileListView
          path={path}
          entries={visibleEntries}
          loading={list.isLoading}
          error={list.error instanceof ApiError ? list.error : null}
          selected={selected}
          renaming={renaming}
          onToggle={toggleSelect}
          onSelect={selectSingle}
          onNavigate={go}
          onGoUp={goUp}
          canGoUp={parentPath(path) !== null}
          onRetry={() => void list.refetch()}
          onRenameCancel={() => setRenaming(null)}
          onRenameCommit={commitRename}
          sort={sort}
          emptyText={mediaOnly ? '该目录没有视频或音乐文件' : '空目录'}
        />
      </div>

      {deleting && (
        <ConfirmDelete
          targets={deleting}
          onCancel={() => setDeleting(null)}
          onConfirm={confirmDelete}
        />
      )}
    </div>
  )
}
