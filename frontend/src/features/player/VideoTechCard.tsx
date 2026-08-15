import type { ReactNode } from 'react'
import { Film, Music2, Play, Route, Subtitles } from 'lucide-react'
import type { AudioTrack, SubtitleTrack, Video } from '../../api/videos'
import { formatBytes, formatDuration } from '../../lib/format'
import {
  audioHandling,
  containerHandling,
  modeMeta,
  reportFor,
  videoHandling,
  type Handling,
  type PlayMode,
} from '../../lib/playability'

const MP4_FAMILY = new Set(['mp4', 'm4v', 'mov', 'qt', '3gp', '3g2'])
function isMp4Family(container?: string): boolean {
  return MP4_FAMILY.has(container?.toLowerCase() ?? '')
}

// toneText 是逐流处理/音轨处理文案的语气色：ok=原生或无损、warn=有损或潜在
// 无声、bad=无法处理。
const toneText: Record<Handling['tone'], string> = {
  ok: 'text-neutral-700',
  warn: 'text-amber-600',
  bad: 'text-red-600',
}

// SectionHeading 是卡片内各小节的统一标题（图标 + 文字）。字号与键值对一致，
// 仅用颜色/字重区分层级：标题最深、键最浅、值居中。
function SectionHeading({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-900">
      {icon}
      {children}
    </div>
  )
}

// LossBadge 把「有损/无损」以徽标形式呈现（无损=流拷贝、有损=重编码），作为
// 布尔值比塞在文案括号里更直观。未定义表示该层不做质量转换，不显示。
function LossBadge({ lossless }: { lossless?: boolean }) {
  if (lossless === undefined) return null
  return (
    <span
      className={
        lossless
          ? 'shrink-0 rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-medium leading-none text-emerald-700'
          : 'shrink-0 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium leading-none text-amber-700'
      }
    >
      {lossless ? '无损' : '有损'}
    </span>
  )
}

// InfoRow 是「技术信息」的键值对行：窄屏纵向堆叠、宽屏两端对齐，值靠右。
// warn 时值用琥珀色强调（分段 MP4 / 非快速启动等提示项）。
function InfoRow({ label, warn, title, children }: { label: string; warn?: boolean; title?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 @sm:flex-row @sm:items-baseline @sm:justify-between @sm:gap-4">
      <dt className="text-neutral-400">{label}</dt>
      <dd className={`text-right ${warn ? 'text-amber-600' : 'text-neutral-800'}`} title={title}>
        {children}
      </dd>
    </div>
  )
}

// PlanRow 是「逐流播放处理」的行：键 + 语气色文案 + 有损/无损徽标。
function PlanRow({ label, tone, text, lossless }: { label: string; tone: Handling['tone']; text: string; lossless?: boolean }) {
  return (
    <div className="flex flex-col gap-0.5 @sm:flex-row @sm:items-start @sm:justify-between @sm:gap-4">
      <dt className="@sm:shrink-0 text-neutral-400">{label}</dt>
      <dd className="flex items-center justify-end gap-1.5 @sm:min-w-0 @sm:flex-1">
        <span className={`text-right ${toneText[tone]}`}>{text}</span>
        <LossBadge lossless={lossless} />
      </dd>
    </div>
  )
}

// TrackRow 是音轨/字幕行的统一 li 骨架：左侧轨名（窄屏堆叠、宽屏截断），右侧
// 处理说明/徽标整体由调用方提供。
function TrackRow({ left, right }: { left: ReactNode; right: ReactNode }) {
  return (
    <li className="flex flex-col gap-0.5 @sm:flex-row @sm:items-center @sm:justify-between @sm:gap-4">
      <span className="min-w-0 text-neutral-800 @sm:truncate">{left}</span>
      {right}
    </li>
  )
}

// VideoTechCard presents the probed technical facts of a video and, for each
// stream, how it will be handled when played in this browser (ADR-006 三层动态
// 流: Direct / Remux / Transcode). "不支持前端解码" is no longer a dead end —
// a codec the browser cannot decode natively is stream-copied (remux, 无损) or
// re-encoded (transcode, 有损) instead, so the card explains per stream which
// tier takes over in natural language a non-technical user can read.
//
// 排版约定：所有正文同一字号（text-sm），仅用颜色/字重区分层级；键值对在容器
// 过窄时（@sm 容器断点，如手机竖屏）纵向堆叠，键在上靠左、值在下靠右，避免
// 单行显示不全被省略。
export function VideoTechCard({
  video,
  subtitleTracks,
  audioTracks,
  mode,
}: {
  video: Video
  subtitleTracks: SubtitleTrack[]
  audioTracks: AudioTrack[]
  mode: PlayMode
}) {
  const report = reportFor(video)
  const meta = modeMeta(mode)
  const banner =
    meta.tone === 'bad'
      ? 'border-red-200 bg-red-50 text-red-700'
      : meta.tone === 'warn'
        ? 'border-amber-200 bg-amber-50 text-amber-800'
        : 'border-emerald-200 bg-emerald-50 text-emerald-700'

  const audioSummary =
    audioTracks.length > 0
      ? `${audioTracks[0].codec?.toUpperCase() || '未知'} · ${audioTracks.length} 条声轨`
      : video.audio_codec
        ? video.audio_codec.toUpperCase()
        : '未探测到'

  const containerPlan = containerHandling(mode)
  const videoPlan = videoHandling(mode, video.codec)
  const audioPlan = video.audio_codec ? audioHandling(mode, video.audio_codec) : null

  return (
    <div className="@container rounded-md border border-neutral-200 bg-white p-4">
      <div className={`rounded border px-3 py-2 ${banner}`}>
        <div className="flex items-center gap-1.5 text-sm font-medium">
          <Play className="size-4" />
          {meta.label}
        </div>
        <p className="mt-0.5 text-xs opacity-90">{meta.text}</p>
      </div>

      <div className="mt-4">
        <SectionHeading icon={<Film className="size-4 text-neutral-400" />}>技术信息</SectionHeading>
        <dl className="mt-2 space-y-1.5 text-sm">
          <InfoRow label="容器">
            {video.container?.toUpperCase() || '未知'}
            {report.mime ? <span className="ml-1.5 font-mono text-neutral-400">{report.mime}</span> : ''}
          </InfoRow>
          <InfoRow label="视频">
            {video.codec?.toUpperCase() || '未知'}
            {video.width && video.height ? ` · ${video.width}×${video.height}` : ''}
            {video.fps ? ` · ${video.fps}fps` : ''}
          </InfoRow>
          <InfoRow label="音频">{audioSummary}</InfoRow>
          <InfoRow label="时长 / 大小">
            {video.duration > 0 ? `${formatDuration(video.duration)} · ` : ''}
            {formatBytes(video.size)}
          </InfoRow>
          {video.segmented && (
            <InfoRow label="分段 MP4" warn>
              是（fMP4，浏览器不可直连）
            </InfoRow>
          )}
          {isMp4Family(video.container) && !video.faststart && (
            <InfoRow label="进度拖动" warn title="moov 位于文件尾部，浏览器需缓冲后才能拖动进度">
              非快速启动
            </InfoRow>
          )}
        </dl>
      </div>

      <div className="mt-4">
        <SectionHeading icon={<Route className="size-4 text-neutral-400" />}>逐流播放处理</SectionHeading>
        <dl className="mt-2 space-y-1.5 text-sm">
          <PlanRow label="容器" {...containerPlan} />
          <PlanRow label="视频" {...videoPlan} />
          {audioPlan ? (
            <PlanRow label="音频" {...audioPlan} />
          ) : (
            <PlanRow label="音频" tone="warn" text="未探测到音频轨（无声或源未重扫）" />
          )}
        </dl>
      </div>

      <div className="mt-4">
        <SectionHeading icon={<Music2 className="size-4 text-neutral-400" />}>音频轨道</SectionHeading>
        {audioTracks.length === 0 ? (
          <p className="mt-2 text-sm text-neutral-400">无音频轨道</p>
        ) : (
          <ul className="mt-2 space-y-1 text-sm">
            {audioTracks.map((t) => {
              const handling = audioHandling(mode, t.codec)
              return (
                <TrackRow
                  key={t.index}
                  left={
                    <>
                      {t.label}
                      {t.index === 0 && (
                        <span className="ml-1.5 rounded bg-neutral-100 px-1 py-0.5 text-[10px] font-medium leading-none text-neutral-500">
                          默认
                        </span>
                      )}
                      <span className="ml-1.5 text-neutral-400">
                        {t.codec?.toUpperCase()}
                        {t.channels ? ` · ${t.channels} 声道` : ''}
                      </span>
                    </>
                  }
                  right={
                    <span className="flex shrink-0 items-center gap-1.5">
                      <span className={toneText[handling.tone]}>{handling.text}</span>
                      <LossBadge lossless={handling.lossless} />
                    </span>
                  }
                />
              )
            })}
          </ul>
        )}
      </div>

      <div className="mt-4">
        <SectionHeading icon={<Subtitles className="size-4 text-neutral-400" />}>字幕轨道</SectionHeading>
        {subtitleTracks.length === 0 ? (
          <p className="mt-2 text-sm text-neutral-400">无字幕轨道</p>
        ) : (
          <ul className="mt-2 space-y-1 text-sm">
            {subtitleTracks.map((t) =>
              t.kind === 'sidecar' ? (
                <TrackRow
                  key="sidecar"
                  left={t.label}
                  right={<span className="shrink-0 text-neutral-500">外部字幕文件 · 播放时直接显示</span>}
                />
              ) : (
                <TrackRow
                  key={`e${t.index}`}
                  left={
                    <>
                      内封轨 {t.index}
                      <span className="ml-1.5 text-neutral-400">{t.codec?.toUpperCase()}</span>
                    </>
                  }
                  right={
                    t.playable === false ? (
                      <span className="shrink-0 text-amber-600">内封位图字幕 · 无法转为文本，只能经格式工厂烧录</span>
                    ) : (
                      <span className="shrink-0 text-neutral-500">内封字幕 · 播放时自动提取显示</span>
                    )
                  }
                />
              ),
            )}
          </ul>
        )}
      </div>
    </div>
  )
}
