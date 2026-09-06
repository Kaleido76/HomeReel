import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, Loader2, Play, Repeat, Volume2 } from 'lucide-react'
import { fetchVideo, fetchVideoPrefs } from '../../api/videos'
import { fetchSeriesDetail } from '../../api/series'
import { formatBytes, formatDuration, formatVolume } from '../../lib/format'
import { playMode, prefetchPlayability } from '../../lib/playability'
import { openFormatVideo, playVideo } from '../../tabs/manager'
import { VideoPlayer } from './VideoPlayer'

// PlayerPane is the player tab's playback view. The PC layout is three zones:
//
//   ┌───────────────────────────────┬──────────────────────────┐
//   │                               │  Playback Memory          │
//   │         Video Player          │  (audio / subtitle / vol) │
//   │         (16:9, flex-1)        ├──────────────────────────┤
//   │                               │  Episode List (scroll)    │
//   │                               │  (watched ✓ / active ▶)   │
//   ├───────────────────────────────┴──────────────────────────┤
//   │  [Prev] [Next]  [Autoplay]                 [reserved]    │
//   └──────────────────────────────────────────────────────────┘
//
// The video area determines the main row's height (16:9 aspect); the sidebar
// stretches to match. On narrow screens the sidebar stacks below the video
// (responsive breakpoint left as an extension point). The bottom controls bar
// holds playback navigation and advanced settings; additional controls will be
// added in the reserved area on the right.
//
// The component receives only videoId and seriesId; navigation (prev/next,
// episode clicks) goes through the manager, so the player tab's URL stays in
// sync. For standalone videos (no seriesId) the sidebar is hidden.
export function PlayerPane({
  videoId,
  seriesId,
}: {
  videoId: string
  seriesId?: string
}) {
  const detail = useQuery({ queryKey: ['video', videoId], queryFn: () => fetchVideo(videoId) })
  const seriesDetail = useQuery({
    queryKey: ['series', seriesId ?? 'none'],
    queryFn: () => fetchSeriesDetail(seriesId!),
    enabled: !!seriesId,
  })
  const prefs = useQuery({
    queryKey: ['prefs', videoId],
    queryFn: () => fetchVideoPrefs(videoId),
  })
  const [autoplay, setAutoplay] = useState(true)

  const seriesMembers = seriesDetail.data?.members
  useEffect(() => {
    if (seriesMembers) prefetchPlayability(seriesMembers)
  }, [seriesMembers])

  const video = detail.data?.video
  const mode = video && detail.data ? playMode(video, detail.data) : 'none'
  const openConvert = () => video && openFormatVideo(video.path)

  const neighbours = useMemo(() => {
    const members = seriesDetail.data?.members ?? []
    const idx = members.findIndex((m) => m.video_id === videoId)
    if (idx < 0) return { prev: undefined, next: undefined }
    return {
      prev: idx > 0 ? members[idx - 1].video_id : undefined,
      next: idx < members.length - 1 ? members[idx + 1].video_id : undefined,
    }
  }, [seriesDetail.data, videoId])

  const go = (id: string) => playVideo(id, seriesId)

  const p = prefs.data?.prefs
  const prefAudio = p?.scope === 'series' ? p.audio_track_name : typeof p?.audio_track === 'number' ? `音轨 ${p.audio_track + 1}` : undefined
  const prefSubtitle = p?.scope === 'series'
    ? (typeof p.subtitle_name === 'string' ? (p.subtitle_name === '' ? '关闭' : p.subtitle_name) : undefined)
    : (typeof p?.subtitle_id === 'string' ? (p.subtitle_id === '' ? '关闭' : p.subtitle_id) : undefined)
  const prefVolume = typeof p?.volume === 'number' ? formatVolume(p.volume, p.muted) : undefined

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
      {/* Main area: video + sidebar.
          Narrow: CSS Grid with two rows — video row is auto-sized (content
          height,不可压缩), sidebar fills the rest and scrolls. This prevents
          the video from being squeezed out when the viewport is short.
          Wide: flex row — video takes remaining width, sidebar fixed 320px.
          Web fullscreen: video expands to cover the entire viewport. */}
      <div className="grid min-h-0 flex-1 grid-rows-[auto_1fr] lg:flex lg:flex-row">
        {/* Video area: aspect-ratio is determined by Vidstack (video's natural ratio,
            typically 16:9). overflow-hidden keeps the Vidstack controls inside. */}
        <div className="relative min-w-0 overflow-hidden bg-black lg:flex-1 lg:shrink-0">
          {mode !== 'none' ? (
            <VideoPlayer
              video={video}
              mode={mode}
              onEnded={autoplay && neighbours.next ? () => go(neighbours.next!) : undefined}
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-3 p-6">
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
        </div>

        {/* Sidebar: always visible. For series playback it shows the full
            metadata + episode list; for standalone videos the episode list
            is empty and the title falls back to the video title. */}
        <div className="flex min-h-0 min-w-0 flex-col overflow-y-auto border-l-0 border-t border-neutral-200 bg-white lg:w-[320px] lg:shrink-0 lg:overflow-y-visible lg:border-l lg:border-t-0">
          {/* Section 1: Title — series name or video title */}
          <div className="flex shrink-0 items-baseline gap-2 border-b border-neutral-200 px-3 py-2.5">
            <p className="min-w-0 truncate text-sm font-medium text-neutral-800" title={seriesDetail.data?.series.name ?? video.title}>
              {seriesDetail.data?.series.name ?? video.title}
            </p>
            {seriesDetail.data && (
              <span className="shrink-0 text-xs text-neutral-400">
                {seriesDetail.data.series.member_count} 集
              </span>
            )}
          </div>

          {/* Section 2: Detailed info */}
          <div className="shrink-0 border-b border-neutral-200">
            <p className="px-3 pt-2.5 pb-1.5 text-sm font-medium text-neutral-700">详细信息</p>
            <div className="px-3 pb-2.5">
              <p className="mb-1.5 text-xs font-medium text-neutral-500">播放习惯</p>
              <dl className="space-y-1 text-xs">
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">音轨</dt>
                  <dd className="min-w-0 truncate text-right text-neutral-700">{prefAudio ?? '默认'}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">字幕</dt>
                  <dd className="min-w-0 truncate text-right text-neutral-700">{prefSubtitle ?? '默认'}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">音量</dt>
                  <dd className="flex shrink-0 items-center gap-1 text-neutral-700">
                    {prefVolume ? (
                      <>
                        <Volume2 className="size-3" />
                        {prefVolume}
                      </>
                    ) : (
                      '默认'
                    )}
                  </dd>
                </div>
              </dl>
            </div>
            <div className="px-3 pb-2.5">
              <p className="mb-1.5 text-xs font-medium text-neutral-500">技术信息</p>
              <dl className="space-y-1 text-xs">
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">容器</dt>
                  <dd className="text-neutral-700">{video.container?.toUpperCase() || '未知'}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">视频</dt>
                  <dd className="text-right text-neutral-700">
                    {video.codec?.toUpperCase() || '未知'}
                    {video.width && video.height ? ` · ${video.width}×${video.height}` : ''}
                    {video.fps ? ` · ${video.fps}fps` : ''}
                  </dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">音频</dt>
                  <dd className="text-neutral-700">{video.audio_codec?.toUpperCase() || '未知'}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-neutral-500">时长 / 大小</dt>
                  <dd className="text-right text-neutral-700">
                    {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
                    {formatBytes(video.size)}
                  </dd>
                </div>
              </dl>
            </div>
          </div>

          {/* Section 3: Episode list — only meaningful for series playback */}
          {seriesDetail.data && (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col">
              <div className="flex shrink-0 items-center justify-between px-3 pt-2.5 pb-1.5">
                <p className="text-sm font-medium text-neutral-700">剧集列表</p>
                <button
                  onClick={() => setAutoplay((v) => !v)}
                  className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors ${
                    autoplay
                      ? 'bg-blue-50 text-blue-600'
                      : 'bg-neutral-100 text-neutral-400'
                  }`}
                >
                  <Repeat className="size-3" />
                  连播
                </button>
              </div>
              <div className="min-h-0 min-w-0 flex-1 overflow-y-auto">
                <ul className="divide-y divide-neutral-100">
                  {seriesDetail.data.members.map((m) => {
                    const active = m.video_id === videoId
                    const watched = m.progress > 0
                    return (
                      <li key={m.video_id}>
                        <button
                          onClick={() => go(m.video_id)}
                          className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors ${
                            active ? 'bg-blue-50' : 'hover:bg-neutral-50'
                          }`}
                        >
                          <span
                            className={`w-5 shrink-0 text-center text-xs font-medium ${
                              active ? 'text-blue-600' : 'text-neutral-400'
                            }`}
                          >
                            {active ? <Play className="mx-auto size-3.5" /> : m.episode_number}
                          </span>
                          <div className="min-w-0 flex-1">
                            <p
                              className={`truncate text-sm ${active ? 'font-medium text-blue-700' : 'text-neutral-700'}`}
                              title={m.episode_title || m.title}
                            >
                              {m.episode_title || m.title}
                            </p>
                          </div>
                          {watched && !active && (
                            <Check className="size-3.5 shrink-0 text-emerald-500" />
                          )}
                          {m.duration > 0 && (
                            <span className="shrink-0 text-xs text-neutral-400">{formatDuration(m.duration)}</span>
                          )}
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
