import { useState } from 'react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../../api/client'
import {
  addPin,
  addSource,
  fetchDisks,
  fetchFilesList,
  fetchPins,
  fetchSources,
  filesCopy,
  filesDelete,
  filesMove,
  filesRename,
  filesRenames,
  markResources,
  removePin,
  removeSource,
  scanSource,
} from '../../api/files'
import { DriveRail } from './DriveRail'
import { Toolbar, type ClipMode, type Clipboard, type ClipboardItem, type SortState } from './Toolbar'
import { FileListView } from './FileListView'
import { ConfirmDelete } from './ConfirmDelete'
import { ToolDrawerShell } from './ToolDrawerShell'
import { ClipboardDrawer } from './ClipboardDrawer'
import { RenameDrawer } from './RenameDrawer'
import { isMediaName } from './fileType'
import { basename, parentPath } from './path'
import { jobsKey } from '../jobs/useJobs'
import { openFormat } from '../../tabs/manager'
import type { ConvertTarget } from '../tools/format/queue'

interface FilesSearch {
  path: string
}

// FilesPage is the generic machine-wide file browser (文件 tab): it
// lists directories by absolute path on demand (nothing is indexed) and offers
// Windows-Explorer-style clipboard operations (cut/copy/paste/rename/delete).
// The current path lives in the URL so refresh/deep-link restores the folder.
export function FilesPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { path } = (location.search ?? {}) as FilesSearch

  const [sort, setSort] = useState<SortState>({ key: 'name', dir: 'asc' })
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [clipboard, setClipboard] = useState<Clipboard | null>(null)
  const [renameTargets, setRenameTargets] = useState<ClipboardItem[] | null>(null)
  const [activeDrawer, setActiveDrawer] = useState<'clipboard' | 'rename' | null>(null)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string[] | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [mediaOnly, setMediaOnly] = useState(false)

  const disks = useQuery({ queryKey: ['files-disks'], queryFn: fetchDisks, staleTime: 60_000 })
  const pins = useQuery({ queryKey: ['files-pins'], queryFn: fetchPins })
  const sources = useQuery({
    queryKey: ['files-sources'],
    queryFn: fetchSources,
    // 轮询以感知扫描完成/离线状态变化
    refetchInterval: 5000,
  })
  const list = useQuery({
    queryKey: ['files-list', path],
    queryFn: () => fetchFilesList(path),
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
    navigate({ to: '/files', search: { path: p } })
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
    if (path) queryClient.invalidateQueries({ queryKey: ['files-list', path] })
    queryClient.invalidateQueries({ queryKey: ['files-pins'] })
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

  function selectAll() {
    setSelected(new Set(visibleEntries.map((e) => e.path)))
  }

  function invertSelection() {
    const all = new Set(visibleEntries.map((e) => e.path))
    setSelected((prev) => {
      const next = new Set<string>()
      for (const p of all) if (!prev.has(p)) next.add(p)
      return next
    })
  }

  // selectedItems snapshots the checked rows into display metadata so drawers
  // keep rendering name/icon even if checkboxes change while they are open.
  function selectedItems(): ClipboardItem[] {
    return Array.from(selected).map((p) => {
      const e = entries.find((x) => x.path === p)
      return e
        ? { path: e.path, name: e.name, is_dir: e.is_dir }
        : { path: p, name: basename(p), is_dir: false }
    })
  }

  function setClipboardMode(mode: ClipMode) {
    if (selected.size === 0) return
    setClipboard({ mode, items: selectedItems() })
    setActiveDrawer('clipboard')
  }

  function openBatchRename() {
    if (selected.size < 2) return
    setRenameTargets(selectedItems())
    setActiveDrawer('rename')
  }

  function closeRenameDrawer() {
    setRenameTargets(null)
    setActiveDrawer(null)
  }

  async function commitRenames(renames: { path: string; newName: string }[]) {
    setError('')
    try {
      const res = await filesRenames(renames)
      if (res.errors && res.errors.length > 0) {
        setError(res.errors.map((e) => `${e.path}: ${e.message}`).join('；'))
      } else {
        flash(`已重命名 ${renames.length} 项`)
      }
      closeRenameDrawer()
      setSelected(new Set())
      invalidate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '批量重命名失败')
    }
  }

  function removeClipboardItem(p: string) {
    if (!clipboard) return
    const items = clipboard.items.filter((i) => i.path !== p)
    if (items.length === 0) {
      setClipboard(null)
      setActiveDrawer(null)
    } else {
      setClipboard({ ...clipboard, items })
    }
  }

  function clearClipboard() {
    setClipboard(null)
    setActiveDrawer(null)
  }

  async function paste() {
    if (!clipboard || !path) return
    setError('')
    try {
      const paths = clipboard.items.map((i) => i.path)
      if (clipboard.mode === 'copy') {
        await filesCopy(paths, path)
      } else {
        await filesMove(paths, path)
      }
      const verb = clipboard.mode === 'copy' ? '复制' : '移动'
      flash(`${verb}已作为后台任务开始，可在顶部任务面板查看进度`)
      // 复制/移动是后台任务，立即刷新任务指示器让顶部图标马上旋转。
      void queryClient.invalidateQueries({ queryKey: jobsKey })
      if (clipboard.mode === 'cut') {
        setClipboard(null)
        setActiveDrawer(null)
        setSelected(new Set())
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '操作失败')
    }
  }

  async function commitRename(p: string, newName: string) {
    setError('')
    try {
      await filesRename(p, newName)
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
      const res = await filesDelete(deleting)
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
      await queryClient.invalidateQueries({ queryKey: ['files-pins'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : pinned ? '取消固定失败' : '固定失败')
    }
  }

  async function toggleSource() {
    if (!path) return
    setError('')
    try {
      if (isSource) {
        if (
          !window.confirm(
            '取消多媒体源标记会将该源下所有已入库的单集与系列从视频库中移除（磁盘文件不受影响）。确认取消？'
          )
        ) {
          return
        }
        await removeSource(path)
        flash('已取消多媒体源标记，其下视频已从库中移除')
        void queryClient.invalidateQueries({ queryKey: ['videos'] })
        void queryClient.invalidateQueries({ queryKey: ['series'] })
      } else {
        const res = await addSource(path)
        flash(res.job_id ? '已标记为多媒体源，开始扫描…' : '已标记为多媒体源')
        if (res.job_id) void queryClient.invalidateQueries({ queryKey: jobsKey })
      }
      await queryClient.invalidateQueries({ queryKey: ['files-sources'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : isSource ? '取消多媒体源失败' : '标记多媒体源失败')
    }
  }

  async function rescanSource(p: string) {
    setError('')
    try {
      await scanSource(p)
      flash('已提交重新扫描')
      void queryClient.invalidateQueries({ queryKey: jobsKey })
      await queryClient.invalidateQueries({ queryKey: ['files-sources'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '重新扫描失败')
    }
  }

  // 标记所选文件夹为手动系列（系列必须位于媒体源内；后端拒绝源外路径）。
  async function markSelected() {
    const paths = !hasDirSelected && entries.some((e) => !e.is_dir && e.is_video) ? [path] : Array.from(selected)
    if (paths.length === 0 || paths.some((p) => !p)) return
    setError('')
    try {
      const res = await markResources(paths, 'series')
      const label = hasDirSelected ? '系列' : '当前目录为系列'
      flash(`已标记 ${paths.length} 个${label}，开始入库…`)
      if (res.job_ids.length > 0) void queryClient.invalidateQueries({ queryKey: jobsKey })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '标记失败')
    }
  }

  // 转到格式工厂：有勾选则把勾选的路径（文件=单集、文件夹=系列）整体移交，
  // 无勾选但当前目录含视频时把当前目录作为系列移交。转换本身在格式工厂页签执行。
  function goFormat() {
    const toTarget = (p: string): ConvertTarget => {
      const e = entries.find((x) => x.path === p)
      return {
        path: p,
        name: e?.name ?? basename(p),
        is_dir: e?.is_dir ?? false,
      }
    }
    const items =
      selectedCount > 0
        ? Array.from(selected).map(toTarget)
        : path && entries.some((e) => !e.is_dir && e.is_convertible)
          ? [{ path, name: basename(path), is_dir: true }]
          : []
    if (items.length > 0) openFormat(items)
  }

  const pinned = pins.data?.pins?.includes(path) ?? false
  const isSource = sources.data?.sources?.some((s) => s.path === path) ?? false
  const selectedCount = selected.size
  // 系列标记：所选条目全部是文件夹时标记所选（系列 ↔ 物理文件夹严格对应）；
  // 否则若当前目录含多媒体，标记当前所在文件夹。
  const hasDirSelected =
    selectedCount > 0 &&
    Array.from(selected).some((p) => {
      const e = entries.find((x) => x.path === p)
      return e?.is_dir
    })
  const canMarkSeries = hasDirSelected
    ? Array.from(selected).every((p) => {
        const e = entries.find((x) => x.path === p)
        return e?.is_dir
      })
    : entries.some((e) => !e.is_dir && e.is_video)
  const canFormat = selectedCount > 0 || entries.some((e) => !e.is_dir && e.is_convertible)

  return (
    <div className="flex h-full min-h-0">
      <DriveRail
        disks={disks.data?.disks ?? []}
        pins={pins.data?.pins ?? []}
        sources={sources.data?.sources ?? []}
        currentPath={path}
        onNavigate={go}
        onRescanSource={rescanSource}
      />

      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Toolbar
          path={path}
          canGoUp={parentPath(path) !== null}
          entryCount={visibleEntries.length}
          selectedCount={selectedCount}
          clipboard={clipboard}
          notice={notice}
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
          onSelectAll={selectAll}
          onInvertSelection={invertSelection}
          onBatchRename={openBatchRename}
          onMarkSeries={() => void markSelected()}
          canMarkSeries={canMarkSeries}
          onFormat={goFormat}
          canFormat={canFormat}
          onPin={togglePin}
          pinned={pinned}
          isSource={isSource}
          onToggleSource={toggleSource}
          mediaOnly={mediaOnly}
          onToggleMedia={() => setMediaOnly((v) => !v)}
        />

        {error && <p className="border-b border-neutral-100 bg-red-50 px-3 py-2 text-sm text-red-600">{error}</p>}

        <FileListView
          path={path}
          entries={visibleEntries}
          loading={list.isLoading}
          error={list.error instanceof ApiError ? list.error : null}
          selected={selected}
          renaming={renaming}
          sort={sort}
          onSortChange={setSort}
          onToggle={toggleSelect}
          onSelect={selectSingle}
          onSelectSet={setSelected}
          onNavigate={go}
          onRetry={() => void list.refetch()}
          onRenameCancel={() => setRenaming(null)}
          onRenameCommit={commitRename}
          emptyText={mediaOnly ? '该目录没有视频或音乐文件' : '空目录'}
        />

        <ToolDrawerShell open={activeDrawer === 'clipboard' && clipboard !== null}>
          {clipboard && (
            <ClipboardDrawer
              items={clipboard.items}
              mode={clipboard.mode}
              onRemove={removeClipboardItem}
              onClear={clearClipboard}
            />
          )}
        </ToolDrawerShell>

        <ToolDrawerShell open={activeDrawer === 'rename' && renameTargets !== null} heightClass="max-h-[26rem]">
          {renameTargets && <RenameDrawer items={renameTargets} onClose={closeRenameDrawer} onApply={commitRenames} />}
        </ToolDrawerShell>
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
