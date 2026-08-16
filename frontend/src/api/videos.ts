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
// backend-computed playability flags and the on-demand source-file status
// (ok | moved | missing, 单集详情同步提示用).
//
// direct_playable is the conservative Range fallback; remux_playable gates the
// container-only MP4 remux (/api/stream/:id/remux, served over Range) and
// transcode_playable gates the on-demand HLS re-encode (/api/stream/:id/hls).
export interface VideoDetailResponse {
  video: Video
  tags: string[]
  series_id?: string
  direct_playable: boolean
  remux_playable: boolean
  transcode_playable: boolean
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
  kind?: 'movie' | 'episode'
  year?: number
  rating?: number
  genre?: string
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

export function clearHistory(id: string): Promise<{ cleared: boolean }> {
  return api<{ cleared: boolean }>(`/api/videos/${id}/history`, { method: 'DELETE' })
}

// PlaybackPrefs is the effective playback selection cache (ADR-006 player
// prefs): the chosen audio track, the chosen subtitle source and the volume.
// scope tells which record the values come from — "series" means the video's
// series shares one memory across every episode (tracks are matched by NAME
// against the current episode's own tracks), "video" the member's own concrete
// selections. The player auto-applies them on every play and only refreshes them
// when the user manually changes a selection. As a rebuildable cache it can be
// cleared from the cache manager page or the detail pages; deleting a row resets
// the video to the browser defaults.
export interface PlaybackPrefs {
  scope: 'series' | 'video'
  audio_track?: number
  subtitle_id?: string
  audio_track_name?: string
  subtitle_name?: string
  volume?: number
  muted?: boolean
  updated_at?: string
}

// VideoPrefsResponse carries the effective prefs plus the video's series context
// (series_id is present whenever the video belongs to a series, so the player
// writes series-scoped on manual changes even before a series record exists).
export interface VideoPrefsResponse {
  prefs: PlaybackPrefs | null
  series_id?: string
}

// PlaybackPrefsPatch is a partial update: only the provided fields are written.
// For a series member the player sends track NAMES (audio_track_name/subtitle_
// name) and volume/muted; subtitle_name is '' when subtitles were turned off.
// Standalone videos send the concrete per-video audio_track/subtitle_id.
export interface PlaybackPrefsPatch {
  audio_track?: number
  subtitle_id?: string
  audio_track_name?: string
  subtitle_name?: string
  volume?: number
  muted?: boolean
}

export function fetchVideoPrefs(id: string): Promise<VideoPrefsResponse> {
  return api<VideoPrefsResponse>(`/api/videos/${id}/prefs`)
}

export function putVideoPrefs(id: string, patch: PlaybackPrefsPatch): Promise<{ saved: boolean }> {
  return api<{ saved: boolean }>(`/api/videos/${id}/prefs`, {
    method: 'PUT',
    body: JSON.stringify(patch),
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

// remuxUrl returns the cached remuxed-MP4 stream URL (ADR-006 修订): served over
// HTTP Range like a native file, generated on first play for browser-decodable
// streams in a foreign container (e.g. MKV h264+aac). audio selects which audio
// track is mapped (default 0).
export function remuxUrl(id: string, audio = 0): string {
  return `/api/stream/${id}/remux${audio > 0 ? `?audio=${audio}` : ''}`
}

// hlsPlaylistUrl returns the VOD playlist URL of the on-demand HLS transcode
// tier. Each play session carries a fresh token so concurrent viewers keep
// separate segment caches (ADR-002 多终端并发); audio selects which audio track
// is transcoded (default 0).
export function hlsPlaylistUrl(id: string, session: string, audio = 0): string {
  const params = new URLSearchParams({ session })
  if (audio > 0) params.set('audio', String(audio))
  return `/api/stream/${id}/hls/playlist.m3u8?${params.toString()}`
}

export function coverUrl(id: string, thumb = false): string {
  return `/api/stream/${id}/cover${thumb ? '?thumb=1' : ''}`
}

export function subtitleUrl(id: string, track?: number): string {
  return `/api/stream/${id}/subtitle${track !== undefined ? `?track=${track}` : ''}`
}

// SubtitleTrack is one subtitle source of a video (a sidecar file next to the
// video, an embedded text stream of the container, or a bitmap track).
// playable tracks can feed the player's <track> menu; a bitmap track (PGS/
// VobSub, playable=false) cannot be converted to text and only works burned
// into the picture by the format factory.
export interface SubtitleTrack {
  kind: 'sidecar' | 'embedded'
  index?: number
  codec?: string
  label: string
  playable?: boolean
}

export function fetchVideoSubtitles(id: string): Promise<{ subtitles: SubtitleTrack[] }> {
  return api<{ subtitles: SubtitleTrack[] }>(`/api/videos/${id}/subtitles`)
}

// AudioTrack is one audio track of a video the player can switch to (multi-track
// containers like 国语/粤语 MKVs). Index is the 0-based audio-stream ordinal,
// passed to the stream endpoints as ?audio=.
export interface AudioTrack {
  index: number
  codec?: string
  language?: string
  label: string
  channels?: number
}

export function fetchVideoAudioTracks(id: string): Promise<{ audio: AudioTrack[] }> {
  return api<{ audio: AudioTrack[] }>(`/api/videos/${id}/audio`)
}
