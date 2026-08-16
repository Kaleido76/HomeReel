import { Zap, Film, Disc3 } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

// ConvertParams mirrors the backend's fservice.ConvertParams: which video codec
// to use (stream copy / h264 / h265), the re-encode quality (CRF), how audio is
// handled (smart = copy only universally-playable codecs else AAC), the AAC
// bitrate, and whether the first subtitle is burned into the picture.
export interface ConvertParams {
  video: 'copy' | 'h264' | 'h265'
  vcrf: number
  audio: 'smart' | 'copy' | 'aac'
  akbps: number
  burn: boolean
}

// The backend normalizes empty/invalid fields to these defaults, so they must
// match the 快速 MP4 preset exactly.
export const DEFAULT_PARAMS: ConvertParams = {
  video: 'copy',
  vcrf: 19,
  audio: 'smart',
  akbps: 192,
  burn: false,
}

export interface ConvertPreset {
  id: string
  label: string
  icon: LucideIcon
  description: string
  params: ConvertParams
}

// CONVERT_PRESETS are the operations-panel tools. Clicking one fills the
// parameter form with its params; any manual change marks the selection as
// "已自定义". All presets target faststart MP4 (the browser-playable format).
export const CONVERT_PRESETS: ConvertPreset[] = [
  {
    id: 'fast',
    label: '快速 MP4',
    icon: Zap,
    description: '无损流拷贝，画质不变',
    params: { ...DEFAULT_PARAMS },
  },
  {
    id: 'h264',
    label: 'H.264 MP4',
    icon: Film,
    description: '重新编码为 H.264，兼容性最好',
    params: { video: 'h264', vcrf: 19, audio: 'smart', akbps: 192, burn: false },
  },
  {
    id: 'h265',
    label: 'H.265 MP4',
    icon: Disc3,
    description: '重新编码为 H.265/HEVC，同等画质体积更小',
    params: { video: 'h265', vcrf: 23, audio: 'smart', akbps: 192, burn: false },
  },
]

export function paramsEqual(a: ConvertParams, b: ConvertParams): boolean {
  return a.video === b.video && a.vcrf === b.vcrf && a.audio === b.audio && a.akbps === b.akbps && a.burn === b.burn
}

// matchPreset returns the preset whose params equal the current selection, or
// null once the user has customized them.
export function matchPreset(params: ConvertParams): ConvertPreset | null {
  return CONVERT_PRESETS.find((p) => paramsEqual(p.params, params)) ?? null
}

const CRF_CHOICES = [
  { value: 16, label: '16 高画质' },
  { value: 19, label: '19 高质量' },
  { value: 23, label: '23 均衡' },
  { value: 28, label: '28 小体积' },
  { value: 32, label: '32 更小' },
]

export function crfChoices(): { value: number; label: string }[] {
  return CRF_CHOICES
}

const KBPS_CHOICES = [128, 192, 256]

export function kbpsChoices(): number[] {
  return KBPS_CHOICES
}
