import { Film, Subtitles } from 'lucide-react'
import type { SubtitleTrack, Video } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import { reportFor } from '../../lib/playability'

const MP4_FAMILY = new Set(['mp4', 'm4v', 'mov', 'qt', '3gp', '3g2'])
function isMp4Family(container?: string): boolean {
  return MP4_FAMILY.has(container?.toLowerCase() ?? '')
}

// VideoTechCard presents the probed technical facts of a video in one place:
// stream codecs with per-stream playability, container/resolution/duration and
// the subtitle track list (sidecar files + embedded text tracks). The play
// banner already says "not playable"; this card explains *which* part cannot be
// decoded, so the user knows whether 格式工厂 will actually fix it.
export function VideoTechCard({ video, subtitleTracks }: { video: Video; subtitleTracks: SubtitleTrack[] }) {
  const report = reportFor(video)
  const issues: string[] = []
  if (!report.containerKnown) {
    issues.push(`容器 ${video.container?.toUpperCase() || '未知'} 无法直接播放`)
  }
  if (report.videoSupported === false && report.containerKnown) {
    issues.push(`视频编码 ${video.codec?.toUpperCase() || '未知'} 不支持前端解码`)
  }
  if (report.audioSupported === false) {
    issues.push(
      report.audioCodec
        ? `音频 ${report.audioCodec.toUpperCase()} 不支持前端解码`
        : '未探测到音频轨（可能是 DTS/PCM，播放可能无声）',
    )
  }
  if (report.decoderSupported === false) {
    issues.push('本机浏览器不支持解码该编码组合（如 HEVC）')
  }

  return (
    <div className="rounded-md border border-neutral-200 bg-white p-4">
      <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-700">
        <Film className="size-4 text-neutral-400" />
        技术信息
      </div>
      <dl className="mt-3 space-y-1.5 text-sm">
        <div className="flex justify-between gap-4">
          <dt className="text-neutral-500">视频</dt>
          <dd className="text-right text-neutral-800">
            {video.codec?.toUpperCase() || '未知'}
            {video.width && video.height ? ` · ${video.width}×${video.height}` : ''}
            {video.fps ? ` · ${video.fps}fps` : ''}
          </dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-neutral-500">音频</dt>
          <dd
            className={
              report.audioSupported === false ? 'text-right font-medium text-amber-600' : 'text-right text-neutral-800'
            }
          >
            {video.audio_codec?.toUpperCase() || '未探测到'}
          </dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-neutral-500">容器</dt>
          <dd className="text-right text-neutral-800">{video.container?.toUpperCase() || '未知'}</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-neutral-500">时长 / 大小</dt>
          <dd className="text-right text-neutral-800">
            {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
            {formatBytes(video.size)}
          </dd>
        </div>
        {isMp4Family(video.container) && !video.faststart && (
          <div className="flex justify-between gap-4">
            <dt className="text-neutral-500">进度拖动</dt>
            <dd className="text-right text-amber-600" title="moov 位于文件尾部，浏览器需缓冲后才能拖动进度">
              非快速启动
            </dd>
          </div>
        )}
      </dl>

      {issues.length > 0 && (
        <div className="mt-3 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          {issues.map((it) => (
            <p key={it} className="py-0.5">
              {it}
            </p>
          ))}
        </div>
      )}

      <div className="mt-4">
        <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-700">
          <Subtitles className="size-4 text-neutral-400" />
          字幕轨道
        </div>
        {subtitleTracks.length === 0 ? (
          <p className="mt-2 text-xs text-neutral-500">无字幕轨道</p>
        ) : (
          <ul className="mt-2 space-y-1 text-sm">
            {subtitleTracks.map((t) => (
              <li key={t.kind === 'embedded' ? `e${t.index}` : 'sidecar'} className="flex items-center justify-between gap-4">
                <span className="truncate text-neutral-800">{t.label}</span>
                <span className="shrink-0 text-xs text-neutral-400">
                  {t.kind === 'sidecar' ? '侧边文件' : `内封轨 ${t.index}`}
                  {t.codec ? ` · ${t.codec.toUpperCase()}` : ''}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
