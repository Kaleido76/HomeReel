import { api } from './client'

export interface Series {
  id: string
  show_id: string
  root_path?: string
  title: string
  name: string
  kind: 'tv' | 'movie'
  season_number: number
  overview?: string
  year?: number
  rating?: number
  genre?: string
  poster_path?: string
  backdrop_path?: string
  metadata_source: string
  member_count: number
  link_count: number
  total_duration: number
}

export interface SeriesMember {
  video_id: string
  title: string
  title_source?: string
  episode_number: number
  episode_title?: string
  duration: number
  thumb_path?: string
  relative_path: string
  progress: number
  codec?: string
  audio_codec?: string
  container?: string
  segmented?: boolean
  direct_playable: boolean
  remux_playable: boolean
  transcode_playable: boolean
}

export interface SeriesLink {
  series_id: string
  linked_id: string
  linked_title: string
  linked_name: string
  sort_index: number
}

export interface SeriesCheck {
  root_exists: boolean
  out_of_sync: boolean
  missing?: string[]
  new?: string[]
}

export interface SeriesDetail {
  series: Series
  members: SeriesMember[]
  links: SeriesLink[]
  check: SeriesCheck
}

export interface SeriesQuery {
  q?: string
  genre?: string
  year?: number
  tags?: string[]
}

export function fetchSeries(query: SeriesQuery = {}): Promise<{ series: Series[] }> {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.genre) params.set('genre', query.genre)
  if (query.year) params.set('year', String(query.year))
  for (const tag of query.tags ?? []) params.append('tag', tag)
  const qs = params.toString()
  return api<{ series: Series[] }>(`/api/series${qs ? `?${qs}` : ''}`)
}

export function fetchSeriesDetail(id: string): Promise<SeriesDetail> {
  return api<SeriesDetail>(`/api/series/${id}`)
}

export function syncSeries(id: string): Promise<{ synced: boolean }> {
  return api<{ synced: boolean }>(`/api/series/${id}/sync`, { method: 'POST' })
}

export function clearSeriesHistory(id: string): Promise<{ cleared: boolean }> {
  return api<{ cleared: boolean }>(`/api/series/${id}/history`, { method: 'DELETE' })
}

export function reorderSeriesMembers(id: string, videoIds: string[]): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/order`, {
    method: 'POST',
    body: JSON.stringify({ video_ids: videoIds }),
  })
}

// resortSeries restores the automatic file-name member order (系列详情「按文件名
// 字典序重新刷新排序」)：清除 sort_manual 并按文件名重绑成员 1..N，手动改过的
// 标题保留。
export function resortSeries(id: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/resort`, { method: 'POST' })
}

// updateSeriesName renames a series. Series are backed 1:1 by a show row (each
// root folder gets its own show, ADR-015), so the existing show metadata PATCH
// carries the rename; it also rebuilds the members' search text.
export function updateSeriesName(showId: string, name: string): Promise<{ show: { id: string; name: string } }> {
  return api(`/api/shows/${showId}`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  })
}

// setSeriesLinks replaces the series' link group with the full desired set
// (方案 B)：勾选集即期望的关联集合，提交后该系列与勾选系列同组互相可见。
export function setSeriesLinks(id: string, seriesIds: string[]): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/links`, {
    method: 'PUT',
    body: JSON.stringify({ series_ids: seriesIds }),
  })
}

export function removeSeriesLink(id: string, linkedId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/links/${linkedId}`, { method: 'DELETE' })
}

export function seriesPosterUrl(id: string): string {
  return `/api/series/${id}/poster`
}

// SeriesPlaybackPrefs is a series' shared playback selection cache (ADR-006
// player prefs 修订): every episode auto-applies the same audio track, subtitle
// and volume. Tracks are stored by NAME (e.g. "简体中文") and resolved against
// each episode's own track list at play time, so a choice made in one episode
// follows the others.
export interface SeriesPlaybackPrefs {
  series_id: string
  audio_track_name?: string
  subtitle_name?: string
  volume?: number
  muted?: boolean
  updated_at: string
}

export function fetchSeriesPrefs(id: string): Promise<{ prefs: SeriesPlaybackPrefs | null }> {
  return api<{ prefs: SeriesPlaybackPrefs | null }>(`/api/series/${id}/prefs`)
}

// clearSeriesPrefs deletes a series' shared playback selection cache (detail
// pages「清除缓存」/ cache manager). After clearing, episodes fall back to their
// own per-video records.
export function clearSeriesPrefs(id: string): Promise<{ cleared: number }> {
  return api<{ cleared: number }>(`/api/series/${id}/prefs`, { method: 'DELETE' })
}
