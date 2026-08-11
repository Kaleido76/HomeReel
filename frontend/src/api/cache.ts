import { api } from './client'

export interface CacheClass {
  files: number
  bytes: number
  orphans: number
  orphan_bytes: number
}

export interface CacheOrphans {
  cover: CacheClass
  thumb: CacheClass
  subtitle: CacheClass
}

export interface SubtitleCacheFile {
  track: number
  name: string
  bytes: number
}

export interface SubtitleCacheGroup {
  video_id: string
  title: string
  show_id?: string
  show_title?: string
  files: SubtitleCacheFile[]
  bytes: number
}

export interface CacheOverview {
  orphans: CacheOrphans
  subtitles: SubtitleCacheGroup[]
}

export function fetchCacheStats(): Promise<CacheOverview> {
  return api<CacheOverview>('/api/cache')
}

export function clearSubtitleCache(videoId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/subtitles/${videoId}`, { method: 'DELETE' })
}

export function clearSubtitleTrack(videoId: string, track: number): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/subtitles/${videoId}/${track}`, { method: 'DELETE' })
}

export function clearAllSubtitleCache(): Promise<{ cleared: number }> {
  return api<{ cleared: number }>('/api/cache?kind=subtitle', { method: 'DELETE' })
}

export function clearOrphanCache(): Promise<{ cleared: number }> {
  return api<{ cleared: number }>('/api/cache/orphans', { method: 'DELETE' })
}
