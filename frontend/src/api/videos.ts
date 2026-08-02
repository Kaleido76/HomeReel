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
  duration: number
  codec: string
  container: string
  width: number
  height: number
  cover_path: string
  thumb_path: string
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
  sort?: 'title' | 'date' | 'duration' | 'name'
  order?: 'asc' | 'desc'
  page?: number
  pageSize?: number
}

// VideoDetailResponse is the shape of GET /api/videos/:id: the video plus the
// backend-computed playability flags (single source of truth, ADR-006).
export interface VideoDetailResponse {
  video: Video
  direct_playable: boolean
  hls_enabled: boolean
}

export interface HistoryEntry {
  video_id: string
  user: string
  progress: number
  updated_at: string
}

export function fetchVideos(query: VideoQuery = {}): Promise<VideoListResult> {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
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

export function fetchHistory(id: string): Promise<{ history: HistoryEntry | null }> {
  return api<{ history: HistoryEntry | null }>(`/api/videos/${id}/history`)
}

export function putHistory(id: string, progress: number): Promise<{ saved: boolean }> {
  return api<{ saved: boolean }>(`/api/videos/${id}/history`, {
    method: 'PUT',
    body: JSON.stringify({ progress }),
  })
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
