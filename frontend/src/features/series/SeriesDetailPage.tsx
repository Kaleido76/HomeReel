import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Calendar, Check, Layers, Link2, Loader2, Pencil, Plus, RefreshCw, Star, X } from 'lucide-react'
import { fetchSeriesDetail, removeSeriesLink, seriesPosterUrl, setSeriesLinks, syncSeries, updateSeriesName } from '../../api/series'
import { SeriesPickerModal } from '../../components/SeriesPickerModal'
import { prefetchPlayability } from '../../lib/playability'
import { openFileLocation } from '../../tabs/manager'
import { SeriesMemberList } from './SeriesMemberList'
import { SeriesProgressCard } from './SeriesProgressCard'

// SeriesDetailPage renders the series detail for the middle column of the
// wide-screen library. It receives the id as a prop (the route is matched by
// LibraryLayout, not rendered through <Outlet/>), so useParams is unnecessary.
//
// The detail response carries an on-demand disk check (check): when the series
// root is missing, a member file vanished, or new files were dropped into the
// folder, a warning is shown with a manual sync action (系列详情同步).
export function SeriesDetailPage({ seriesId }: { seriesId: string }) {
  const id = seriesId
  const queryClient = useQueryClient()
  const detail = useQuery({ queryKey: ['series', id], queryFn: () => fetchSeriesDetail(id) })
  const [editingName, setEditingName] = useState(false)
  const [nameInput, setNameInput] = useState('')
  const [pickingLink, setPickingLink] = useState(false)

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['series', id] })

  // 数据到达即预收集全部成员的可播放性（进入页面时集中判定一次，成员行渲染
  // 直接命中缓存），避免几十个成员逐个 createElement + canPlayType 的累积开销。
  const detailMembers = detail.data?.members
  useEffect(() => {
    if (detailMembers) prefetchPlayability(detailMembers)
  }, [detailMembers])

  const removeLink = useMutation({
    mutationFn: (linkedId: string) => removeSeriesLink(id, linkedId),
    onSuccess: invalidate,
  })
  const saveLinks = useMutation({
    // 关联系列（方案 B）：勾选集 = 期望的关联集合，全量 PUT 替换——
    // 该系列与勾选系列同组互相可见，取消勾选即不再关联。
    mutationFn: (pickedIds: string[]) => setSeriesLinks(id, pickedIds),
    onSuccess: () => {
      setPickingLink(false)
      invalidate()
    },
  })
  const sync = useMutation({
    mutationFn: () => syncSeries(id),
    onSuccess: () => {
      invalidate()
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
    },
  })

  const rename = useMutation({
    mutationFn: ({ showId, name }: { showId: string; name: string }) => updateSeriesName(showId, name),
    onSuccess: () => {
      setEditingName(false)
      invalidate()
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
    },
  })

  if (detail.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError || !detail.data) {
    return (
      <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {detail.error?.message}
      </div>
    )
  }

  const { series, members, links, check } = detail.data
  const linkedIds = new Set(links.map((l) => l.linked_id))
  const rootPath = series.root_path

  const commitName = () => {
    const t = nameInput.trim()
    setEditingName(false)
    if (!t || t === series.name) return
    rename.mutate({ showId: series.show_id, name: t })
  }

  return (
    <>
      <div className="space-y-4 p-4 sm:p-5">
      {check?.out_of_sync && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3">
          <p className="flex items-center gap-1.5 text-sm text-amber-800">
            <AlertTriangle className="size-4 shrink-0" />
            该系列与磁盘上的文件夹不一致。
          </p>
          {!check.root_exists && <p className="mt-1 text-sm text-amber-700">系列根目录已不存在（可能被改名或删除）。</p>}
          {(check.missing ?? []).length > 0 && (
            <p className="mt-1 text-sm text-amber-700">已消失的成员：{(check.missing ?? []).join('、')}</p>
          )}
          {(check.new ?? []).length > 0 && (
            <p className="mt-1 text-sm text-amber-700">尚未入库的新文件：{(check.new ?? []).join('、')}</p>
          )}
          <button
            onClick={() => sync.mutate()}
            disabled={sync.isPending}
            className="mt-2 flex items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-40"
          >
            <RefreshCw className="size-3.5" /> {sync.isPending ? '同步中…' : '同步'}
          </button>
        </div>
      )}

      <div className="relative overflow-hidden rounded-xl border border-neutral-200">
        <div className="relative flex flex-col gap-4 bg-white p-5 sm:flex-row sm:gap-5">
          <img
            src={seriesPosterUrl(series.id)}
            alt={series.name}
            className="mx-auto h-52 w-36 shrink-0 rounded-lg border border-neutral-200 object-cover sm:mx-0"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-600">系列剧集</span>
              <span className="flex items-center gap-1 text-xs text-neutral-400">
                <Layers className="size-3.5" /> {series.member_count} 个成员
              </span>
            </div>
            <div className="mt-2 flex min-w-0 items-center gap-2">
              {editingName ? (
                <div className="flex w-full min-w-0 items-center gap-2">
                  <input
                    value={nameInput}
                    onChange={(e) => setNameInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') commitName()
                      if (e.key === 'Escape') setEditingName(false)
                    }}
                    autoFocus
                    className="w-full min-w-0 rounded-md border border-neutral-300 bg-white px-2 py-1 text-2xl font-semibold text-neutral-900 outline-none focus:border-blue-600"
                  />
                  <button onClick={commitName} title="保存" className="shrink-0 rounded p-1 text-blue-600 hover:bg-blue-50">
                    <Check className="size-4" />
                  </button>
                  <button onClick={() => setEditingName(false)} title="取消" className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100">
                    <X className="size-4" />
                  </button>
                </div>
              ) : (
                <div className="flex min-w-0 flex-1 items-center gap-2">
                  <h1 className="min-w-0 truncate text-2xl font-semibold text-neutral-900" title={series.name}>
                    {series.name}
                  </h1>
                  <button
                    onClick={() => {
                      setNameInput(series.name)
                      setEditingName(true)
                    }}
                    title="编辑系列名称"
                    className="shrink-0 rounded p-1 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-900"
                  >
                    <Pencil className="size-4" />
                  </button>
                </div>
              )}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-neutral-500">
              {series.rating ? (
                <span className="flex items-center gap-1">
                  <Star className="size-4 fill-neutral-300 text-neutral-400" /> {series.rating.toFixed(1)}
                </span>
              ) : null}
              {series.year ? (
                <span className="flex items-center gap-1">
                  <Calendar className="size-4" /> {series.year}
                </span>
              ) : null}
              {series.genre ? <span>{series.genre}</span> : null}
            </div>
            {series.overview ? <p className="mt-3 line-clamp-3 text-sm leading-relaxed text-neutral-600">{series.overview}</p> : null}
            {rootPath ? (
              <button
                onClick={() => openFileLocation(rootPath)}
                title={`在文件页定位系列根目录：${rootPath}`}
                className="mt-3 inline-block max-w-full truncate font-mono text-xs text-neutral-400 transition-colors hover:text-blue-600 hover:underline"
              >
                {rootPath}
              </button>
            ) : null}
          </div>
        </div>
      </div>

      <SeriesProgressCard seriesId={id} members={members} />

      <div className="rounded-xl border border-neutral-200 bg-white p-4">
        <SeriesMemberList seriesId={id} members={members} />
      </div>

      <div className="rounded-xl border border-neutral-200 bg-white p-4">
        <h3 className="mb-3 text-sm font-medium text-neutral-700">
          关联系列
          <span className="ml-2 text-xs font-normal text-neutral-400">关联无名称，仅用于一起展示与跳转</span>
        </h3>
        {links.length === 0 && <p className="mb-3 text-sm text-neutral-400">暂无关联。可在下方手动添加。</p>}
        <div className="flex flex-wrap gap-2">
          {links.map((l) => (
            <div key={l.linked_id} className="flex items-center gap-1.5 rounded-md border border-neutral-200 py-1 pl-3 pr-1.5">
              <Link to="/series/$id" params={{ id: l.linked_id }} className="text-sm font-medium text-neutral-700 hover:text-blue-600">
                {l.linked_name}
              </Link>
              <button
                onClick={() => removeLink.mutate(l.linked_id)}
                disabled={removeLink.isPending}
                className="rounded p-1 text-neutral-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-40"
                title="移除关联"
              >
                <X className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
        <button
          onClick={() => setPickingLink(true)}
          disabled={saveLinks.isPending}
          className="mt-3 flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          <Plus className="size-4" /> 管理关联
        </button>
        {pickingLink && (
          <SeriesPickerModal
            multiple
            title="管理关联系列"
            titleIcon={<Link2 className="size-4 text-neutral-500" />}
            excludeIds={[id]}
            initialSelectedIds={[...linkedIds]}
            onConfirm={(selected) => saveLinks.mutate(selected.map((s) => s.id))}
            onClose={() => setPickingLink(false)}
          />
        )}
      </div>
    </div>
    </>
  )
}
