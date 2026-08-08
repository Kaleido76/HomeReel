import { api } from './client'

// Types shared with the generic machine-wide file browser (文件 tab).
// All paths are absolute host paths; file contents are never indexed.

export interface Disk {
  path: string
  label: string
  type: 'fixed' | 'removable'
  total: number
  free: number
}

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mtime: number // unix seconds
  is_video: boolean
}

export interface FileListResult {
  path: string
  entries: FileEntry[]
}

export interface OpResult {
  done: number
  errors?: { path: string; message: string }[]
}

export function fetchDisks(): Promise<{ disks: Disk[] }> {
  return api<{ disks: Disk[] }>('/api/disks')
}

export function fetchFilesList(path: string): Promise<FileListResult> {
  const params = new URLSearchParams({ path })
  return api<FileListResult>(`/api/files/list?${params.toString()}`)
}

export function filesCopy(paths: string[], dest: string): Promise<{ ok: boolean; job_id: string }> {
  return api<{ ok: boolean; job_id: string }>('/api/files/copy', {
    method: 'POST',
    body: JSON.stringify({ paths, dest }),
  })
}

export function filesMove(paths: string[], dest: string): Promise<{ ok: boolean; job_id: string }> {
  return api<{ ok: boolean; job_id: string }>('/api/files/move', {
    method: 'POST',
    body: JSON.stringify({ paths, dest }),
  })
}

export function filesRename(path: string, newName: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/files/rename', {
    method: 'POST',
    body: JSON.stringify({ path, newName }),
  })
}

export function filesDelete(paths: string[]): Promise<OpResult> {
  return api<OpResult>('/api/files/delete', {
    method: 'POST',
    body: JSON.stringify({ paths }),
  })
}

export function fetchPins(): Promise<{ pins: string[] }> {
  return api<{ pins: string[] }>('/api/files/pins')
}

export function addPin(path: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/files/pins', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

export function removePin(path: string): Promise<{ ok: boolean }> {
  const params = new URLSearchParams({ path })
  return api<{ ok: boolean }>(`/api/files/pins?${params.toString()}`, { method: 'DELETE' })
}

// Multimedia source markers: lightweight persistent "this directory is mostly
// media" declarations that feed the video library. available=false means the
// root is currently unreachable (temporarily offline — its library rows are
// left untouched); scanning=true while a scan job is queued/running.
export interface MediaSource {
  id: string
  path: string
  created_at: string
  last_scan_at?: string
  available: boolean
  scanning: boolean
}

export function fetchSources(): Promise<{ sources: MediaSource[] }> {
  return api<{ sources: MediaSource[] }>('/api/files/sources')
}

export function addSource(path: string): Promise<{ source: MediaSource; job_id: string }> {
  return api<{ source: MediaSource; job_id: string }>('/api/files/sources', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

export function removeSource(path: string): Promise<{ removed: boolean }> {
  const params = new URLSearchParams({ path })
  return api<{ removed: boolean }>(`/api/files/sources?${params.toString()}`, { method: 'DELETE' })
}

export function scanSource(path: string): Promise<{ job_id: string }> {
  return api<{ job_id: string }>('/api/files/sources/scan', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}
