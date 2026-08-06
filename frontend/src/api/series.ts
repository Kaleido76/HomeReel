import { api } from './client'

export interface Series {
  id: string
  show_id: string
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
  episode_number: number
  episode_title?: string
  duration: number
  thumb_path?: string
  relative_path: string
  storage_id: string
  storage_available: boolean
  progress: number
}

export interface SeriesLink {
  series_id: string
  linked_id: string
  linked_title: string
  linked_name: string
  sort_index: number
}

export interface SeriesDetail {
  series: Series
  members: SeriesMember[]
  links: SeriesLink[]
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

export function fetchSeriesLinks(id: string): Promise<{ links: SeriesLink[] }> {
  return api<{ links: SeriesLink[] }>(`/api/series/${id}/links`)
}

export function addSeriesLink(id: string, linkedId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/links`, {
    method: 'POST',
    body: JSON.stringify({ series_id: linkedId }),
  })
}

export function removeSeriesLink(id: string, linkedId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/series/${id}/links/${linkedId}`, { method: 'DELETE' })
}

export function seriesPosterUrl(id: string): string {
  return `/api/series/${id}/poster`
}
