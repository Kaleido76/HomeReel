// playability.ts — 前后端协同能力判定（ADR-006，2026-08 修订）。
//
// 「运行期固化」的判定机制：前端把 probe 出的容器/视频编码/音频编码映射成
// MIME + codecs 串，再用 HTMLMediaElement.canPlayType() 核对目标机器（本机
// 浏览器 + 系统解码能力）能否直接播放。判定结果决定走哪一层：
//   direct    → 浏览器原生解码 → HTTP Range 直连（/api/stream/{id}）
//   remux     → 视频可解但容器不兼容（MKV h264+aac，音频可流拷贝）→ 后端流拷贝
//               成缓存 MP4，再走 Range 直连（/api/stream/{id}/remux）
//   transcode → 编码不兼容（HEVC/rmvb/DTS 等）或音频不可拷贝（AC3/EAC3/DTS/PCM）
//               → 后端按需转码成 HLS（/api/stream/{id}/hls/playlist.m3u8）
//   none      → 以上皆不可 → 引导格式工厂转换
//
// 后端返回的 direct_playable / remux_playable / transcode_playable 是能力标志：
// direct 是保守 fallback（当 canPlayType 探测不可用时兜底），remux/transcode 是
// 动态流层的可用性门槛。
//
// —— 性能优化（判定规则不变）——
// canPlayType 的结果在页面生命周期内稳定（浏览器解码能力不会中途变化），因此
// 按「编码组合」做 memoize 缓存，并复用同一个 <video> 元素而非每次 createElement。
// 页面进入 / 数据到达时可用 prefetchPlayability 一次性预收集一批条目（如系列
// 详情页的全部成员），渲染时直接命中缓存，避免逐条 createElement + canPlayType
// 的累积开销。缓存为进程内 Map，随页面刷新重建（安装解码扩展后刷新即重新判定）。

// PlayabilityInput is the probe metadata a playability decision needs. Video
// and SeriesMember both carry these fields.
export interface PlayabilityInput {
  container?: string
  codec?: string
  audio_codec?: string
  segmented?: boolean
}

// containerMimes maps a probed container to a MIME the browser can hand to
// canPlayType. Containers without a browser <video src> path (avi/wmv/ts/flv/
// mpeg/… ) are simply absent and therefore never playable.
//
// mov/qt are mapped to video/mp4, not video/quicktime: mp4 and mov are the same
// box family that Chromium demuxes with one code path, and ffprobe reports the
// whole family as "mov,mp4,m4a,3gp,3g2,mj2" whose first token is "mov" — a real
// .mp4 file is thus stored with container "mov". canPlayType('video/quicktime;
// codecs=…') returns empty in Chromium, which would misjudge every converted/
// scanned mp4 as non-playable; video/mp4 always resolves for the box family.
const containerMimes: Record<string, string> = {
  mp4: 'video/mp4',
  m4v: 'video/mp4',
  mov: 'video/mp4',
  qt: 'video/mp4',
  matroska: 'video/x-matroska',
  mkv: 'video/x-matroska',
  webm: 'video/webm',
  ogg: 'video/ogg',
  ogv: 'video/ogg',
}

// videoTokens maps a probed video codec to an RFC 6381 codecs token. Unknown
// codecs (mpeg2/realvideo/wmv3/vc1/…) are absent and thus never playable.
const videoTokens: Record<string, string> = {
  h264: 'avc1.42E01E',
  avc1: 'avc1.42E01E',
  avc3: 'avc3.42E01E',
  hevc: 'hvc1.1.6.L93.0',
  hvc1: 'hvc1.1.6.L93.0',
  vp8: 'vp8',
  vp9: 'vp09.00.10.08',
  av1: 'av01.0.00M.08',
  theora: 'theora',
}

// audioTokens maps a probed audio codec to its codecs token. AC3/EAC3, DTS and
// lossless PCM are absent: Chromium and Firefox decode none of these inside a
// <video src> (no Dolby decoder), so a file with such audio would play silently.
// These codecs are re-encoded to AAC by the HLS transcode tier (or 格式工厂)
// instead.
const audioTokens: Record<string, string> = {
  aac: 'mp4a.40.2',
  mp3: 'mp4a.69',
  opus: 'opus',
  vorbis: 'vorbis',
  flac: 'flac',
}

// PlayabilityReport is the decision behind canPlay, broken down per stream so
// the detail page can explain *which* part blocks playback (container / video
// codec / audio codec) instead of a bare "not playable".
//
// - containerKnown: container maps to a browser <video src> MIME.
// - videoSupported / audioSupported: the codec has a decodable token. audio is
//   null when there is no audio track (or the decision never reached it).
// - decoderSupported: what canPlayType returned for the whole combination; null
//   when canPlayType threw or was never reached (the caller then falls back to
//   the backend's conservative flag).
// - playable: the final decision (decoderSupported when reached, else false).
export interface PlayabilityReport {
  containerKnown: boolean
  mime?: string
  videoCodec?: string
  videoSupported: boolean
  audioCodec?: string
  audioSupported: boolean | null
  decoderSupported: boolean | null
  playable: boolean
}

// —— 缓存与复用（优化，判定规则与 canPlay 完全一致）——

const MAX_CACHE = 512
const cache = new Map<string, PlayabilityReport>()

let sharedVideoEl: HTMLVideoElement | null = null

function videoEl(): HTMLVideoElement {
  if (!sharedVideoEl) {
    sharedVideoEl = document.createElement('video')
  }
  return sharedVideoEl
}

function cacheKey(media: PlayabilityInput): string {
  return `${media.container ?? ''}|${media.codec ?? ''}|${media.audio_codec ?? ''}|${media.segmented ? '1' : '0'}`
}

function remember(key: string, report: PlayabilityReport) {
  if (cache.size >= MAX_CACHE) cache.clear()
  cache.set(key, report)
}

// computeReport is the pure decision. A report whose decision never reached
// canPlayType (异常环境) keeps decoderSupported null and is not cached, so a
// later attempt can still succeed.
function computeReport(media: PlayabilityInput): PlayabilityReport {
  const report: PlayabilityReport = {
    containerKnown: false,
    videoCodec: media.codec,
    videoSupported: false,
    audioCodec: media.audio_codec,
    audioSupported: null,
    decoderSupported: null,
    playable: false,
  }
  if (media.segmented) return report
  const mime = containerMimes[media.container?.toLowerCase() ?? '']
  if (!mime) return report
  report.containerKnown = true
  report.mime = mime
  const vTok = media.codec ? videoTokens[media.codec.toLowerCase()] : undefined
  report.videoSupported = vTok !== undefined
  if (!vTok) return report
  const aTok = media.audio_codec ? audioTokens[media.audio_codec.toLowerCase()] : undefined
  if (media.audio_codec) {
    report.audioSupported = aTok !== undefined
    if (!aTok) return report
  } else if (mime === 'video/x-matroska') {
    // MKV 未探测到音频编码（audio_codec 为空）：无法确认其音轨能否解码——
    // DTS/PCM 等 Chromium 不支持会无声，而空值可能只是源未重扫、audio_codec
    // 尚未填充。保守判不可播放；重扫源填充后，有声轨的按编码正确判定。
    report.audioSupported = false
    return report
  }
  const codecs = [vTok]
  if (aTok) codecs.push(aTok)
  try {
    report.decoderSupported = videoEl().canPlayType(`${mime}; codecs="${codecs.join(',')}"`) !== ''
  } catch {
    return report
  }
  report.playable = report.decoderSupported
  return report
}

// reportFor is the cached entry point used by canPlay and prefetchPlayability.
export function reportFor(media: PlayabilityInput): PlayabilityReport {
  const key = cacheKey(media)
  const hit = cache.get(key)
  if (hit) return hit
  const report = computeReport(media)
  if (report.decoderSupported !== null) remember(key, report)
  return report
}

// canPlay is the single runtime playability decision. backendPlayable is the
// backend's conservative fallback, used only when canPlayType itself throws.
export function canPlay(media: PlayabilityInput, backendPlayable: boolean): boolean {
  const report = reportFor(media)
  if (report.playable) return true
  if (report.decoderSupported === null) return backendPlayable
  return false
}

// PlayMode is the streaming tier a video will actually play through (ADR-006
// 修订): direct Range, container-only remux MP4, on-demand HLS transcode, or
// none (format-factory fallback).
export type PlayMode = 'direct' | 'remux' | 'transcode' | 'none'

// PlayabilityBackendFlags are the backend-computed capability gates returned by
// the video/series APIs. direct is the conservative fallback for when the
// runtime canPlayType probe is unavailable; remux/transcode gate the dynamic
// tiers.
export interface PlayabilityBackendFlags {
  direct_playable?: boolean
  remux_playable?: boolean
  transcode_playable?: boolean
}

// playMode decides which streaming tier to use. The runtime canPlayType report
// is authoritative for direct playability; when it is unavailable the backend's
// conservative direct flag falls back. Otherwise the file falls through to the
// cheapest dynamic tier the backend offers (remux before transcode).
export function playMode(media: PlayabilityInput, backend: PlayabilityBackendFlags): PlayMode {
  const report = reportFor(media)
  if (report.playable) return 'direct'
  if (report.decoderSupported === null && backend.direct_playable) return 'direct'
  if (backend.remux_playable) return 'remux'
  if (backend.transcode_playable) return 'transcode'
  return 'none'
}

// —— 逐层处理说明（详情页技术卡片用）——
//
// 三层动态流（ADR-006 修订）下，「能否播放」不再是非黑即白：除 none 外，每个
// 流都会被某层处理。下列帮助函数把一个流在前端播放时的具体处理翻译成可读说明
// 与语气（ok=原生/无损，warn=有损重编码或潜在无声，bad=无法处理），供详情页
// 逐流展示，也方便播放异常时拿着元信息排查。

export interface Handling {
  text: string
  tone: 'ok' | 'warn' | 'bad'
  // lossless 描述该流是否无损处理：true=流拷贝（无损）、false=重编码（有损）；
  // 缺省表示该层不做质量转换（原生直连 / 无法播放），不显示质量徽标。
  lossless?: boolean
}

// ModeMeta 是每种播放方式的一句话说明（标题 + 副文案 + 语气）。
export interface ModeMeta extends Handling {
  label: string
}

export function modeMeta(mode: PlayMode): ModeMeta {
  switch (mode) {
    case 'direct':
      return {
        label: '直接播放',
        text: '本机浏览器可直接播放，无需转换，进度条全片可拖动',
        tone: 'ok',
      }
    case 'remux':
      return {
        label: '重封装播放',
        text: '容器不兼容但编码本机可解，视频流无损重封装为 MP4 后播放；首次播放生成缓存，之后秒开',
        tone: 'ok',
      }
    case 'transcode':
      return {
        label: '转码播放',
        text: '编码或音频不兼容，按需转码后播放；起播快，大段拖动可能需等待转码',
        tone: 'warn',
      }
    case 'none':
      return {
        label: '无法在线播放',
        text: '未配置 ffmpeg 或源文件不可达，需先用格式工厂转换为 MP4',
        tone: 'bad',
      }
  }
}

// containerHandling 描述容器这一层会如何处理（自然语言，供详情页逐流展示）。
export function containerHandling(mode: PlayMode): Handling {
  switch (mode) {
    case 'direct':
      return { text: '本机浏览器原生支持，直连播放', tone: 'ok' }
    case 'remux':
      return { text: '本机浏览器不支持此容器，后端重封装为 MP4 后播放', tone: 'ok' }
    case 'transcode':
      return { text: '不保留原容器，按需转码为 HLS 分片播放', tone: 'warn' }
    case 'none':
      return { text: '无法在线播放，需格式工厂转换', tone: 'bad' }
  }
}

// videoHandling 描述视频流会如何处理。remux 层只对 h264 家族流拷贝（无损）。
export function videoHandling(mode: PlayMode, codec?: string): Handling {
  const name = codec?.toUpperCase() || '未知编码'
  switch (mode) {
    case 'direct':
      return { text: `本机浏览器可解码 ${name}，原生播放`, tone: 'ok' }
    case 'remux':
      return { text: `本机可解码 ${name}，流拷贝重封装`, tone: 'ok', lossless: true }
    case 'transcode':
      return { text: '重编码为 H.264', tone: 'warn', lossless: false }
    case 'none':
      return { text: `无法解码（${name}）`, tone: 'bad' }
  }
}

// audioHandling 描述音频流会如何处理。remux 层仅 aac/mp3 可流拷贝；其余编码
// 拷贝后浏览器可能无声（Chromium/Firefox 无 Dolby 解码器），故单独警告。
export function audioHandling(mode: PlayMode, codec?: string): Handling {
  const name = codec?.toLowerCase()
  switch (mode) {
    case 'direct':
      return { text: '本机浏览器可解码，原生播放', tone: 'ok' }
    case 'remux':
      if (name === 'aac' || name === 'mp3') {
        return { text: '流拷贝', tone: 'ok', lossless: true }
      }
      return { text: '可能无声（无 Dolby 解码器）', tone: 'warn', lossless: true }
    case 'transcode':
      return { text: '重编码为 AAC', tone: 'warn', lossless: false }
    case 'none':
      return { text: '无法解码', tone: 'bad' }
  }
}

// prefetchPlayability 批量预收集一批条目（页面进入 / 数据到达时调用，如系列
// 详情页的全部成员），同步填充缓存，使后续渲染直接命中、不再逐个探测。
export function prefetchPlayability(items: PlayabilityInput[]): void {
  for (const media of items) reportFor(media)
}
