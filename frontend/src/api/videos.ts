import { api } from './client'

export interface Video {
  id: string
  source_id?: string
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
  segmented?: boolean
  faststart?: boolean
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
  desc?: string
  genre?: string
  year?: number
  kind?: 'movie' | 'episode'
  tags?: string[]
  showId?: string
  ungrouped?: boolean
  sort?: 'title' | 'date' | 'duration' | 'name' | 'rating'
  order?: 'asc' | 'desc'
  page?: number
  pageSize?: number
}

// VideoDetailResponse is the shape of GET /api/videos/:id: the video plus the
// backend-computed playability fallback and the on-demand source-file status
// (ok | moved | missing, 单集详情同步提示用).
export interface VideoDetailResponse {
  video: Video
  tags: string[]
  series_id?: string
  direct_playable: boolean
  source_status: 'ok' | 'moved' | 'missing'
  new_path?: string
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
  tags?: string[]
}

export interface TagCount {
  tag: string
  count: number
}

export function fetchVideos(query: VideoQuery = {}): Promise<VideoListResult> {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.desc) params.set('desc', query.desc)
  if (query.genre) params.set('genre', query.genre)
  if (query.year) params.set('year', String(query.year))
  if (query.kind) params.set('kind', query.kind)
  for (const tag of query.tags ?? []) params.append('tag', tag)
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

export function deleteVideo(id: string): Promise<{ deleted: boolean }> {
  return api<{ deleted: boolean }>(`/api/videos/${id}`, { method: 'DELETE' })
}

export function refreshVideo(id: string): Promise<{ queued: boolean }> {
  return api<{ queued: boolean }>(`/api/videos/${id}/refresh`, { method: 'POST' })
}

export function syncVideo(id: string): Promise<{ synced: boolean }> {
  return api<{ synced: boolean }>(`/api/videos/${id}/sync`, { method: 'POST' })
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

export function coverUrl(id: string, thumb = false): string {
  return `/api/stream/${id}/cover${thumb ? '?thumb=1' : ''}`
}

export function subtitleUrl(id: string, track?: number): string {
  return `/api/stream/${id}/subtitle${track !== undefined ? `?track=${track}` : ''}`
}

// SubtitleTrack is one subtitle source the player can pick from (a sidecar file
// next to the video, or an embedded text subtitle stream of the container).
export interface SubtitleTrack {
  kind: 'sidecar' | 'embedded'
  index?: number
  codec?: string
  label: string
}

export function fetchVideoSubtitles(id: string): Promise<{ subtitles: SubtitleTrack[] }> {
  return api<{ subtitles: SubtitleTrack[] }>(`/api/videos/${id}/subtitles`)
}
