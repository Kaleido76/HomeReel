export function formatDuration(sec: number): string {
  const s = Math.round(sec)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = s % 60
  const mm = h > 0 ? String(m).padStart(2, '0') : String(m)
  const ss = String(r).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${m}:${ss}`
}

export function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`
}

// formatEta renders a rough remaining-time estimate for background tasks.
export function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return '少于 1 分钟'
  const totalMin = Math.round(seconds / 60)
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  if (h > 0) return m > 0 ? `约 ${h} 小时 ${m} 分` : `约 ${h} 小时`
  return `约 ${m} 分钟`
}

// formatVolume renders a remembered playback volume (0..1) as a percentage,
// appending a muted marker when the player was muted.
export function formatVolume(volume: number, muted?: boolean): string {
  return `${Math.round(volume * 100)}%${muted ? '（静音）' : ''}`
}
