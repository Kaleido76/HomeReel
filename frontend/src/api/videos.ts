import { api } from './client'

export interface Video {
  id: string
  storage_id: string
  file_id: string
  relative_path: string
  path: string
  size: number
  mtime: number
  title: string
  kind: 'movie' | 'episode'
  description: string
  duration: number
  codec: string
  audio_codec?: string
  container: string
  width: number
  height: number
  fps?: number
  file_size?: number
  cover_path: string
  thumb_path: string
  backdrop_path?: string
  show_id?: string
  season_number?: number
  episode_number?: number
  episode_title?: string
  year?: number
  rating?: number
  genre?: string
  overview?: string
  studio?: string
  cast_text?: string
  metadata_source: string
  created_at: string
  updated_at: string
  last_scanned_at: string
}

export interface VideoListResult {
  videos: Video[]
  total: number
  page: number
  page_size: number
}

export interface VideoQuery {
  q?: string
  kind?: 'movie' | 'episode'
  tag?: string
  showId?: string
  ungrouped?: boolean
  sort?: 'title' | 'date' | 'duration' | 'name' | 'rating'
  order?: 'asc' | 'desc'
  page?: number
  pageSize?: number
}

// VideoDetailResponse is the shape of GET /api/videos/:id: the video plus the
// backend-computed playability flags (single source of truth, ADR-006).
export interface VideoDetailResponse {
  video: Video
  tags: string[]
  series_id?: string
  direct_playable: boolean
  hls_enabled: boolean
}

export interface HistoryEntry {
  video_id: string
  user: string
  progress: number
  updated_at: string
}

export interface VideoPatch {
  title?: string
  description?: string
  kind?: 'movie' | 'episode'
  year?: number
  rating?: number
  genre?: string
  overview?: string
  studio?: string
  cast_text?: string
  show_id?: string
  season_number?: number
  episode_number?: number
  episode_title?: string
  tags?: string[]
}

export interface TagCount {
  tag: string
  count: number
}

export function fetchVideos(query: VideoQuery = {}): Promise<VideoListResult> {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.kind) params.set('kind', query.kind)
  if (query.tag) params.set('tag', query.tag)
  if (query.showId) params.set('showId', query.showId)
  if (query.ungrouped) params.set('ungrouped', '1')
  if (query.sort) params.set('sort', query.sort)
  if (query.order) params.set('order', query.order)
  if (query.page && query.page > 1) params.set('page', String(query.page))
  if (query.pageSize) params.set('pageSize', String(query.pageSize))
  const qs = params.toString()
  return api<VideoListResult>(`/api/videos${qs ? `?${qs}` : ''}`)
}

export function fetchVideo(id: string): Promise<VideoDetailResponse> {
  return api<VideoDetailResponse>(`/api/videos/${id}`)
}

export function updateVideo(id: string, patch: VideoPatch): Promise<{ video: Video }> {
  return api<{ video: Video }>(`/api/videos/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  })
}

export function deleteVideo(id: string, deleteFile = false): Promise<{ deleted: boolean }> {
  return api<{ deleted: boolean }>(`/api/videos/${id}${deleteFile ? '?deleteFile=true' : ''}`, {
    method: 'DELETE',
  })
}

export function refreshVideo(id: string): Promise<{ queued: boolean }> {
  return api<{ queued: boolean }>(`/api/videos/${id}/refresh`, { method: 'POST' })
}

export function uploadVideoCover(id: string, file: File): Promise<{ cover_path: string }> {
  const form = new FormData()
  form.append('file', file)
  return api<{ cover_path: string }>(`/api/videos/${id}/cover`, { method: 'POST', body: form })
}

export function fetchHistory(id: string): Promise<{ history: HistoryEntry | null }> {
  return api<{ history: HistoryEntry | null }>(`/api/videos/${id}/history`)
}

export function putHistory(id: string, progress: number): Promise<{ saved: boolean }> {
  return api<{ saved: boolean }>(`/api/videos/${id}/history`, {
    method: 'PUT',
    body: JSON.stringify({ progress }),
  })
}

export function fetchTags(): Promise<{ tags: TagCount[] }> {
  return api<{ tags: TagCount[] }>('/api/tags')
}

export function searchVideos(q: string): Promise<{ videos: Video[] }> {
  return api<{ videos: Video[] }>(`/api/search?q=${encodeURIComponent(q)}`)
}

export function streamUrl(id: string): string {
  return `/api/stream/${id}`
}

export function hlsUrl(id: string): string {
  return `/api/stream/${id}/hls/master.m3u8`
}

export function coverUrl(id: string, thumb = false): string {
  return `/api/stream/${id}/cover${thumb ? '?thumb=1' : ''}`
}

export function subtitleUrl(id: string): string {
  return `/api/stream/${id}/subtitle`
}
