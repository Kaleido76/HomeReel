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
  remux: CacheClass
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

// CacheRemuxGroup is one video's cached remux MP4s (default + per-audio-track
// outputs). Reported per video so the UI can attribute the remux space to the
// owning series / episode.
export interface CacheRemuxGroup {
  video_id: string
  title: string
  show_id?: string
  show_title?: string
  files: number
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
  remuxes: CacheRemuxGroup[]
  prefs: PlaybackPrefsCacheEntry[]
  series_prefs: SeriesPrefsCacheEntry[]
}

export function fetchCacheStats(): Promise<CacheOverview> {
  return api<CacheOverview>('/api/cache')
}

export function clearSubtitleCache(videoId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/subtitles/${videoId}`, { method: 'DELETE' })
}

export function clearOrphanCache(): Promise<{ cleared: number }> {
  return api<{ cleared: number }>('/api/cache/orphans', { method: 'DELETE' })
}

// clearVideoRemux deletes one video's cached remux MP4s (the standalone-cache
// detail「清理 Remux」).
export function clearVideoRemux(videoId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/remux/${videoId}`, { method: 'DELETE' })
}

// clearSeriesRemux clears the cached remux MP4s of every member of a series in
// one request (series-level「清理 Remux」).
export function clearSeriesRemux(seriesId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/series/${seriesId}/remux`, { method: 'DELETE' })
}

// clearSeriesSubtitles clears the extracted-subtitle cache of every member of a
// series in one request (series-level「清理字幕」).
export function clearSeriesSubtitles(seriesId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/series/${seriesId}/subtitles`, { method: 'DELETE' })
}

// pregenSubtitles enqueues a background job that pre-extracts the given videos'
// embedded text subtitles into the cache, so playback finds them ready (used for
// a lone standalone video or the standalone-cache section).
export function pregenSubtitles(videoIds: string[]): Promise<{ job_id: string }> {
  return api<{ job_id: string }>('/api/cache/pregen', {
    method: 'POST',
    body: JSON.stringify({ video_ids: videoIds }),
  })
}

// pregenSeriesSubtitles enqueues the same pre-extraction job for a whole series
// (series-level「预生成缓存」; the backend resolves members and dedups by series).
export function pregenSeriesSubtitles(seriesId: string): Promise<{ job_id: string }> {
  return api<{ job_id: string }>('/api/cache/pregen', {
    method: 'POST',
    body: JSON.stringify({ series_id: seriesId }),
  })
}

export function clearPrefs(videoId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/cache/prefs/${videoId}`, { method: 'DELETE' })
}

// clearSeriesPrefs deletes one series' shared playback selection cache (the
// cache manager reuses the series detail page's endpoint).
export function clearSeriesPrefs(seriesId: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/series/${seriesId}/prefs`, { method: 'DELETE' })
}
