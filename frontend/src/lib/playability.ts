// playability.ts — 前后端协同能力判定（ADR-006，2026-08 修订）。
//
// 「运行期固化」的判定机制：前端把 probe 出的容器/视频编码/音频编码映射成
// MIME + codecs 串，再用 HTMLMediaElement.canPlayType() 核对目标机器（本机
// 浏览器 + 系统解码能力）能否直接播放。能 → Range 直连；不能 → 引导格式工厂
// 转换。没有 Live/HLS 转码路径（后端 HLS 模块已整体移除）。
//
// 后端 /api/videos/{id} 返回的 direct_playable 是保守 fallback：当 canPlayType
// 探测不可用（异常环境）时兜底，不作为主判据。
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

// audioTokens maps a probed audio codec to its codecs token. DTS and lossless
// PCM are absent because Chromium decodes neither inside a <video src> — a file
// with such audio must be re-encoded (格式工厂 turns it into AAC).
const audioTokens: Record<string, string> = {
  aac: 'mp4a.40.2',
  mp3: 'mp4a.69',
  opus: 'opus',
  vorbis: 'vorbis',
  flac: 'flac',
  ac3: 'ac-3',
  eac3: 'ec-3',
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

// prefetchPlayability 批量预收集一批条目（页面进入 / 数据到达时调用，如系列
// 详情页的全部成员），同步填充缓存，使后续渲染直接命中、不再逐个探测。
export function prefetchPlayability(items: PlayabilityInput[]): void {
  for (const media of items) reportFor(media)
}
