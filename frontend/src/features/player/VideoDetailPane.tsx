import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Play, RefreshCw, Trash2 } from 'lucide-react'
import { ApiError } from '../../api/client'
import { clearPrefs as clearVideoPrefs } from '../../api/cache'
import { clearSeriesPrefs } from '../../api/series'
import {
  deleteVideo,
  fetchVideo,
  fetchVideoAudioTracks,
  fetchVideoPrefs,
  fetchVideoSubtitles,
  syncVideo,
} from '../../api/videos'
import { playMode } from '../../lib/playability'
import { openFormatVideo } from '../../tabs/manager'
import { PlaybackHistoryCard } from '../player/PlaybackHistoryCard'
import { PlaybackPrefsCard } from '../player/PlaybackPrefsCard'
import { PlaybackProgressSection } from '../player/PlaybackProgressSection'
import { VideoMetaPanel } from '../player/VideoMetaPanel'
import { VideoTechCard } from '../player/VideoTechCard'

// VideoDetailPane is the single-video detail shown in the middle column. It is
// deliberately compact: a play action, the editable metadata panel and a
// technical summary line. The heavy player lives in the right-hand column
// (PlayerPane), which opens on play.
//
// playHref is resolved by the caller from the column stack: a detail opened
// inside a series plays within the series (/series/:id/play/:videoId), a
// standalone detail plays via /library/video/:id/play.
//
// seriesScoped marks a detail already opened inside a series column (the caller
// renders the top 「返回系列」 bar). When a video belongs to a series but is
// reached standalone (home/search cards or a /library/video/:id deep link), the
// pane auto-redirects to the series-scoped URL (/series/:id/video/:videoId) so
// there is never a standalone detail that needs a manual "所属系列" jump — the
// way back is always the series context's top bar.
//
// The detail page also checks the source file on demand (单集详情检查): a file
// that was renamed/moved (found by file_id) shows a sync warning; a file that
// is gone shows a Not-Found warning with the option to remove the record (the
// removal only deletes metadata, never the file).
export function VideoDetailPane({
  videoId,
  playHref,
  seriesScoped = false,
}: {
  videoId: string
  playHref: string
  seriesScoped?: boolean
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const detail = useQuery({ queryKey: ['video', videoId], queryFn: () => fetchVideo(videoId) })
  // Shares the ['subtitles', id] cache with VideoPlayer, so opening the player
  // after this pane does not re-fetch the track list.
  const subtitles = useQuery({ queryKey: ['subtitles', videoId], queryFn: () => fetchVideoSubtitles(videoId) })
  // Shares the ['audio', id] cache with VideoPlayer (multi-track containers).
  const audioTracks = useQuery({ queryKey: ['audio', videoId], queryFn: () => fetchVideoAudioTracks(videoId) })
  // 播放选择记忆（ADR-006 player prefs 修订）：与播放器共享同一条 ['prefs', id]
  // 缓存。系列成员展示「来自系列」的有效值，清除时删系列记录（删后回退到单集
  // 自己的记录）；独立视频展示并清除自己的记录。
  const prefs = useQuery({ queryKey: ['prefs', videoId], queryFn: () => fetchVideoPrefs(videoId) })
  const clearPrefsMutation = useMutation({
    mutationFn: () => {
      const p = prefs.data?.prefs
      if (p?.scope === 'series' && prefs.data?.series_id) return clearSeriesPrefs(prefs.data.series_id)
      return clearVideoPrefs(videoId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['prefs', videoId] })
      const sid = prefs.data?.series_id
      if (sid) queryClient.invalidateQueries({ queryKey: ['series-prefs', sid] })
    },
  })

  // 单集详情永远带 series 上下文：视频属于某系列但当前以独立详情打开时，自动
  // 补齐 series 段（replace，不留历史记录），返回系列的入口统一为系列详情列的
  // 顶部「返回系列」条。
  const seriesId = detail.data?.series_id
  const needsSeriesRedirect = !seriesScoped && !!seriesId
  useEffect(() => {
    if (needsSeriesRedirect) {
      void navigate({ href: `/series/${seriesId}/video/${videoId}`, replace: true })
    }
  }, [needsSeriesRedirect, seriesId, videoId, navigate])

  if (detail.isLoading || needsSeriesRedirect) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (detail.isError) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {detail.error.message}
      </div>
    )
  }

  if (!detail.data) return null
  const video = detail.data.video
  const status = detail.data.source_status
  // Playback tier decided at runtime (ADR-006 修订): direct Range, container-only
  // remux MP4, or on-demand HLS transcode; 'none' falls back to the format
  // factory. The heavy player derives the same mode again from its own fetch.
  const mode = playMode(video, detail.data)

  const p = prefs.data?.prefs
  const prefsFromSeries = p?.scope === 'series'
  const prefAudio = prefsFromSeries
    ? typeof p?.audio_track_name === 'string'
      ? p.audio_track_name
      : undefined
    : typeof p?.audio_track === 'number'
      ? audioTracks.data?.audio[p.audio_track]?.label
      : undefined
  const prefSubtitle = prefsFromSeries
    ? typeof p?.subtitle_name === 'string'
      ? p.subtitle_name === ''
        ? '关闭'
        : p.subtitle_name
      : undefined
    : typeof p?.subtitle_id === 'string'
      ? p.subtitle_id === ''
        ? '关闭'
        : p.subtitle_id === 'sidecar'
          ? '侧边文件'
          : `内封轨 ${p.subtitle_id.replace(/^e/, '')}`
      : undefined
  const prefVolume = typeof p?.volume === 'number' ? `${Math.round(p.volume * 100)}%${p.muted ? '（静音）' : ''}` : undefined

  async function doSync() {
    setError('')
    try {
      await syncVideo(video.id)
      queryClient.invalidateQueries({ queryKey: ['video', video.id] })
      queryClient.invalidateQueries({ queryKey: ['videos'] })
      queryClient.invalidateQueries({ queryKey: ['series'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '同步失败')
    }
  }

  async function remove() {
    if (!window.confirm('该单集的源文件已不存在。移除将删除其已入库元数据，磁盘文件不受影响。')) return
    setError('')
    try {
      await deleteVideo(video.id)
      queryClient.invalidateQueries({ queryKey: ['videos'] })
      queryClient.invalidateQueries({ queryKey: ['series'] })
      navigate({ to: '/library' })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '移除失败')
    }
  }

  return (
    <div className="space-y-4 p-4 sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold text-neutral-900">详情</h1>
        {mode !== 'none' && (
          <button
            onClick={() => navigate({ href: playHref })}
            className="flex items-center gap-1.5 rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
          >
            <Play className="size-4" /> 播放
          </button>
        )}
      </div>

      {mode === 'none' && (
        <div className="flex items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 p-3">
          <div className="min-w-0">
            <p className="text-sm font-medium text-amber-800">
              该文件无法在线播放（{video.container?.toUpperCase() || '未知容器'} · {video.codec || '未知编码'}）
            </p>
            <p className="mt-0.5 text-xs text-amber-700">
              此环境既不支持直连播放，动态流转换也不可用（未配置 ffmpeg 或源文件不可达）。请用格式工厂转换。
            </p>
          </div>
          <button
            onClick={() => openFormatVideo(video.path)}
            className="flex shrink-0 items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
          >
            <Play className="size-4" /> 格式工厂转换
          </button>
        </div>
      )}

      {status === 'moved' && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3">
          <p className="text-sm text-amber-800">
            源文件已改名或移动（{detail.data.new_path ? `现位于 ${detail.data.new_path}` : '位置已变化'}）。
            点击同步以更新库中的路径与归属。
          </p>
          <button
            onClick={() => void doSync()}
            className="mt-2 flex items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
          >
            <RefreshCw className="size-3.5" /> 同步
          </button>
        </div>
      )}

      {status === 'missing' && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3">
          <p className="text-sm text-red-700">源文件不存在（可能已被移动或删除）。</p>
          <button
            onClick={() => void remove()}
            className="mt-2 flex items-center gap-1.5 rounded bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700"
          >
            <Trash2 className="size-3.5" /> 移除这个单集
          </button>
        </div>
      )}

      <div className="rounded-md border border-neutral-200 bg-white p-4">
        <VideoMetaPanel video={video} initialTags={detail.data.tags ?? []} />
      </div>

      <PlaybackHistoryCard
        progress={<PlaybackProgressSection videoId={videoId} duration={video.duration} />}
        memory={
          <PlaybackPrefsCard
            audio={prefAudio}
            subtitle={prefSubtitle}
            volume={prefVolume}
            note={prefsFromSeries ? '来自系列' : undefined}
            hasPrefs={!!p}
            clearing={clearPrefsMutation.isPending}
            onClear={() => clearPrefsMutation.mutate()}
          />
        }
      />

      <VideoTechCard
        video={video}
        subtitleTracks={subtitles.data?.subtitles ?? []}
        audioTracks={audioTracks.data?.audio ?? []}
        mode={mode}
      />

      {error && <p className="text-xs text-red-600">{error}</p>}
    </div>
  )
}
