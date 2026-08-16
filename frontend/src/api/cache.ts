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

// PlaybackPrefsCacheEntry is one video's playback selection cache (audio track /
// subtitle / volume) as shown by the cache manager. Clearing it resets the video
// to the browser defaults on the next play.
export interface PlaybackPrefsCacheEntry {
  video_id: string
  title: string
  show_id?: string
  show_title?: string
  audio_track?: number
  subtitle_id?: string
  volume?: number
  muted?: boolean
  updated_at: string
}

// SeriesPrefsCacheEntry is one series' shared playback selection cache as shown
// by the cache manager. Tracks are stored by name (ADR-006 player prefs 修订).
export interface SeriesPrefsCacheEntry {
  series_id: string
  title: string
  audio_track_name?: string
  subtitle_name?: string
  volume?: number
  muted?: boolean
  updated_at: string
}

export interface CacheOverview {
  orphans: CacheOrphans
  subtitles: SubtitleCacheGroup[]
  prefs: PlaybackPrefsCacheEntry[]
  series_prefs: SeriesPrefsCacheEntry[]
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

export function clearPrefs(videoId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/prefs/${videoId}`, { method: 'DELETE' })
}

// clearSeriesPrefs deletes one series' shared playback selection cache (the
// cache manager reuses the series detail page's endpoint).
export function clearSeriesPrefs(seriesId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/series/${seriesId}/prefs`, { method: 'DELETE' })
}

export function clearAllPrefs(): Promise<{ cleared: number }> {
  return api<{ cleared: number }>('/api/cache/prefs', { method: 'DELETE' })
}
