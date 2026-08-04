import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Calendar, Layers, Loader2, Play, Plus, Star, X } from 'lucide-react'
import { addSeriesLink, fetchSeries, fetchSeriesDetail, removeSeriesLink, seriesPosterUrl } from '../../api/series'
import { formatDuration } from '../../lib/format'

// SeriesDetailPage renders the series detail for the middle column of the
// wide-screen library. It receives the id as a prop (the route is matched by
// LibraryLayout, not rendered through <Outlet/>), so useParams is unnecessary.
export function SeriesDetailPage({ seriesId }: { seriesId: string }) {
  const id = seriesId
  const queryClient = useQueryClient()
  const detail = useQuery({ queryKey: ['series', id], queryFn: () => fetchSeriesDetail(id) })
  const allSeries = useQuery({ queryKey: ['series'], queryFn: fetchSeries })
  const [pick, setPick] = useState('')

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['series', id] })

  const addLink = useMutation({
    mutationFn: (linkedId: string) => addSeriesLink(id, linkedId),
    onSuccess: () => {
      setPick('')
      invalidate()
    },
  })
  const removeLink = useMutation({
    mutationFn: (linkedId: string) => removeSeriesLink(id, linkedId),
    onSuccess: invalidate,
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

  const { series, members, links } = detail.data
  const linkedIds = new Set(links.map((l) => l.linked_id))
  const candidates = (allSeries.data?.series ?? []).filter((s) => s.id !== id && !linkedIds.has(s.id))

  const maxEp = members.reduce((m, x) => Math.max(m, x.episode_number), 0)
  const rows: { ep: number; member?: (typeof members)[number] }[] = []
  for (let i = 1; i <= maxEp; i++) {
    rows.push({ ep: i, member: members.find((m) => m.episode_number === i) })
  }

  return (
    <div className="space-y-4 p-4 sm:p-5">
      <div className="relative overflow-hidden rounded-xl border border-neutral-200">
        <div className="relative flex gap-5 bg-white p-5">
          <img
            src={seriesPosterUrl(series.id)}
            alt={series.name}
            className="h-52 w-36 shrink-0 rounded-lg border border-neutral-200 object-cover"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="rounded bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-600">
                {series.kind === 'movie' ? '电影部' : '系列剧集'}
              </span>
              <span className="flex items-center gap-1 text-xs text-neutral-400">
                <Layers className="size-3.5" /> {series.member_count} 个成员
              </span>
            </div>
            <h1 className="mt-2 text-2xl font-semibold text-neutral-900">{series.name}</h1>
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
          </div>
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 bg-white p-4">
        <h3 className="mb-3 text-sm font-medium text-neutral-700">
          {series.kind === 'movie' ? '本部分集' : '本系列剧集'}
          <span className="ml-2 text-xs font-normal text-neutral-400">
            {members.length} 集{maxEp > members.length ? `（编号到第 ${maxEp} 集，存在缺失）` : ''}
          </span>
        </h3>
        <div className="divide-y divide-neutral-100">
          {rows.map(({ ep, member }) =>
            member ? (
              <div key={member.video_id} className="flex items-center gap-3 py-2.5">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded border border-neutral-200 bg-neutral-50 text-sm font-medium text-neutral-500">
                  {ep}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-neutral-800">{member.episode_title || member.title}</p>
                  <p className="mt-0.5 truncate font-mono text-xs text-neutral-400">{member.relative_path}</p>
                  {member.duration > 0 && member.progress > 0 && member.progress < member.duration - 20 && (
                    <div className="mt-1.5 h-1 w-full max-w-xs overflow-hidden rounded-sm bg-neutral-100">
                      <div
                        className="h-full rounded-sm bg-blue-600"
                        style={{ width: `${Math.min(100, (member.progress / member.duration) * 100)}%` }}
                      />
                    </div>
                  )}
                </div>
                <span className="shrink-0 text-xs text-neutral-400">
                  {member.duration > 0 ? formatDuration(member.duration) : ''}
                </span>
                <div className="flex shrink-0 items-center gap-2">
                  <Link
                    to="/series/$id/play/$videoId"
                    params={{ id: seriesId, videoId: member.video_id }}
                    className="flex shrink-0 items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
                  >
                    <Play className="size-3.5" /> {member.progress > 0 ? '续播' : '播放'}
                  </Link>
                  <Link
                    to="/series/$id/video/$videoId"
                    params={{ id: seriesId, videoId: member.video_id }}
                    className="flex shrink-0 items-center gap-1.5 rounded border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50"
                  >
                    详情
                  </Link>
                </div>
              </div>
            ) : (
              <div key={`gap-${ep}`} className="flex items-center gap-3 py-2.5 opacity-50">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-dashed border-neutral-300 text-sm font-medium text-neutral-300">
                  {ep}
                </div>
                <p className="text-sm text-neutral-300">第 {ep} 集缺失</p>
              </div>
            ),
          )}
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 bg-white p-4">
        <h3 className="mb-3 text-sm font-medium text-neutral-700">
          关联系列
          <span className="ml-2 text-xs font-normal text-neutral-400">关联无名称，仅用于一起展示与跳转</span>
        </h3>
        {links.length === 0 && <p className="mb-3 text-sm text-neutral-400">暂无关联。同一标题下的季/部会自动关联，也可手动添加。</p>}
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
        {candidates.length > 0 && (
          <div className="mt-3 flex items-center gap-2">
            <select
              value={pick}
              onChange={(e) => setPick(e.target.value)}
              className="rounded-md border border-neutral-300 bg-white px-2.5 py-1.5 text-sm text-neutral-700 outline-none focus:border-blue-600"
            >
              <option value="">添加关联…</option>
              {candidates.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <button
              onClick={() => addLink.mutate(pick)}
              disabled={!pick || addLink.isPending}
              className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
            >
              <Plus className="size-4" /> 关联
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
