import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ChevronLeft, ChevronRight, Loader2, Play, Repeat } from 'lucide-react'
import { fetchVideo } from '../../api/videos'
import { coverUrl } from '../../api/videos'
import { fetchSeriesDetail } from '../../api/series'
import { formatBytes, formatDuration } from '../../lib/format'
import { playMode, prefetchPlayability } from '../../lib/playability'
import { openFormatVideo } from '../../tabs/manager'
import { Tooltip } from '../../components/Tooltip'
import { VideoPlayer } from './VideoPlayer'

// PlayerPane is the right-hand column of the wide-screen library. Layout, top
// to bottom:
//   - a slim toolbar with the "exit playback" action (back to detail state)
//   - the player, filling the whole remaining height (aspect-ratio auto) so the
//     control bar sits in the bottom black margin instead of over the subtitles
//   - a compact info bar (title + duration + resolution)
//   - prev/next + autoplay controls
//   - a subtle "接下来播放" (up next) list with the next ~3 series members
//
// Prev/next and the up-next list come from the series' ordered members;
// standalone videos get neither. Autoplay plays the next member on ended.
//
// The navigation targets are resolved by the caller from the column stack, not
// from video attributes: exitHref is the player column's parent (pop target)
// and goHref builds the prev/next/up-next URLs. A player pushed on a series
// detail exits back to the series and stays inside it; a standalone player
// exits back to its video detail.
export function PlayerPane({
  videoId,
  exitHref,
  goHref,
}: {
  videoId: string
  exitHref: string
  goHref: (id: string) => string
}) {
  const navigate = useNavigate()
  const detail = useQuery({ queryKey: ['video', videoId], queryFn: () => fetchVideo(videoId) })
  const seriesDetail = useQuery({
    queryKey: ['series', detail.data?.series_id ?? 'none'],
    queryFn: () => fetchSeriesDetail(detail.data!.series_id!),
    enabled: !!detail.data?.series_id,
  })
  const [autoplay, setAutoplay] = useState(true)

  // 播放栏载入系列成员后预收集其可播放性，上下集/「接下来播放」跳转时直接
  // 命中缓存，避免切换时逐个探测。
  const seriesMembers = seriesDetail.data?.members
  useEffect(() => {
    if (seriesMembers) prefetchPlayability(seriesMembers)
  }, [seriesMembers])

  const video = detail.data?.video
  // Playback tier decided at runtime (ADR-006 修订); the decision chain lives in
  // lib/playability.ts and is shared with the detail pane.
  const mode = video && detail.data ? playMode(video, detail.data) : 'none'
  const openConvert = () => video && openFormatVideo(video.path)

  // neighbours and the up-next list in the series' member order (by episode number)
  const { neighbours, upNext } = useMemo(() => {
    const members = seriesDetail.data?.members ?? []
    const idx = members.findIndex((m) => m.video_id === videoId)
    if (idx < 0) return { neighbours: { prev: undefined, next: undefined }, upNext: [] }
    return {
      neighbours: {
        prev: idx > 0 ? members[idx - 1].video_id : undefined,
        next: idx < members.length - 1 ? members[idx + 1].video_id : undefined,
      },
      upNext: members.slice(idx + 1, idx + 4),
    }
  }, [seriesDetail.data, videoId])

  const exit = () => navigate({ href: exitHref })
  const go = (id: string) => navigate({ href: goHref(id) })

  if (detail.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError || !video) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {detail.error?.message ?? '视频不存在'}
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-200 bg-white px-3 py-2">
        <button
          onClick={exit}
          className="flex items-center gap-1.5 rounded px-2 py-1 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
        >
          <ArrowLeft className="size-4" /> 退出播放
        </button>
        <span className="min-w-0 flex-1 truncate text-xs text-neutral-400" title={video.title}>
          {video.title}
        </span>
      </div>

      {mode !== 'none' ? (
        <div className="min-h-0 flex-1 overflow-hidden bg-black">
          <VideoPlayer video={video} mode={mode} onEnded={autoplay && neighbours.next ? () => go(neighbours.next!) : undefined} />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 bg-neutral-50 p-6">
          <p className="text-sm text-neutral-700">
            该文件无法在线播放（{video.container?.toUpperCase() || '未知容器'} · {video.codec || '未知编码'}）
          </p>
          <p className="max-w-md text-center text-xs text-neutral-400">
            此环境既不支持直连播放，动态流转换也不可用（未配置 ffmpeg 或源文件不可达）。请用格式工厂转换为
            MP4 后再观看。转换不会修改原文件。
          </p>
          <button
            onClick={openConvert}
            className="flex items-center gap-1.5 rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
          >
            <Play className="size-4" /> 格式工厂转换
          </button>
        </div>
      )}

      <div className="flex shrink-0 flex-wrap items-center gap-3 border-t border-neutral-200 bg-white px-4 py-2.5">
        <p className="min-w-0 flex-1 truncate text-sm font-medium text-neutral-800" title={video.title}>
          {video.title}
        </p>
        <span className="shrink-0 text-xs text-neutral-400">
          {video.width && video.height ? `${video.width}×${video.height} · ` : ''}
          {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
          {formatBytes(video.size)}
        </span>
        {detail.data?.series_id ? (
          <Link
            to="/series/$id"
            params={{ id: detail.data.series_id }}
            className="shrink-0 text-xs text-blue-600 hover:underline"
          >
            所属系列
          </Link>
        ) : null}
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-2 border-t border-neutral-100 bg-white px-4 py-2.5">
        <Tooltip content="上一集">
          <button
            onClick={() => neighbours.prev && go(neighbours.prev)}
            disabled={!neighbours.prev}
            className="flex items-center gap-1 rounded border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
          >
            <ChevronLeft className="size-4" /> 上一集
          </button>
        </Tooltip>
        <Tooltip content="下一集">
          <button
            onClick={() => neighbours.next && go(neighbours.next)}
            disabled={!neighbours.next}
            className="flex items-center gap-1 rounded border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
          >
            下一集 <ChevronRight className="size-4" />
          </button>
        </Tooltip>
        <span className="ml-auto flex items-center gap-1.5 text-sm text-neutral-600">
          <Repeat className="size-4" />
          <label className="flex cursor-pointer items-center gap-1.5">
            <input
              type="checkbox"
              checked={autoplay}
              onChange={(e) => setAutoplay(e.target.checked)}
              className="accent-blue-600"
            />
            自动连播
          </label>
        </span>
      </div>

      {upNext.length > 0 && (
        <div className="shrink-0 border-t border-neutral-100 bg-white px-4 py-3">
          <p className="mb-2 text-xs font-medium text-neutral-500">接下来播放</p>
          <ul className="space-y-1">
            {upNext.map((m) => (
              <li key={m.video_id}>
                <button
                  onClick={() => go(m.video_id)}
                  className="flex w-full items-center gap-3 rounded px-2 py-1.5 text-left transition-colors hover:bg-neutral-50"
                >
                  <div className="relative h-12 w-20 shrink-0 overflow-hidden rounded border border-neutral-200 bg-neutral-100">
                    {m.thumb_path ? (
                      <img src={coverUrl(m.video_id, true)} alt={m.title} loading="lazy" className="h-full w-full object-cover" />
                    ) : (
                      <div className="flex h-full w-full items-center justify-center text-neutral-300">
                        <Play className="size-4" />
                      </div>
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm text-neutral-700" title={m.episode_title || m.title}>
                      {m.episode_title || m.title}
                    </p>
                    <p className="truncate text-xs text-neutral-400">第 {m.episode_number} 集</p>
                  </div>
                  {m.duration > 0 && <span className="shrink-0 text-xs text-neutral-400">{formatDuration(m.duration)}</span>}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
