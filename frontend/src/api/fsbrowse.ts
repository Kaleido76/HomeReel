import { api } from './client'

// Types shared with the generic machine-wide file browser (文件（新） tab).
// All paths are absolute host paths; file contents are never indexed.

export interface Disk {
  path: string
  label: string
  type: 'fixed' | 'removable'
  total: number
  free: number
}

export interface Fs2Entry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mtime: number // unix seconds
  is_video: boolean
}

export interface Fs2ListResult {
  path: string
  entries: Fs2Entry[]
}

export interface OpResult {
  done: number
  errors?: { path: string; message: string }[]
}

export function fetchDisks(): Promise<{ disks: Disk[] }> {
  return api<{ disks: Disk[] }>('/api/disks')
}

export function fetchFs2List(path: string): Promise<Fs2ListResult> {
  const params = new URLSearchParams({ path })
  return api<Fs2ListResult>(`/api/fs2/list?${params.toString()}`)
}

export function fs2Copy(paths: string[], dest: string): Promise<{ ok: boolean; job_id: string }> {
  return api<{ ok: boolean; job_id: string }>('/api/fs2/copy', {
    method: 'POST',
    body: JSON.stringify({ paths, dest }),
  })
}

export function fs2Move(paths: string[], dest: string): Promise<{ ok: boolean; job_id: string }> {
  return api<{ ok: boolean; job_id: string }>('/api/fs2/move', {
    method: 'POST',
    body: JSON.stringify({ paths, dest }),
  })
}

export function fs2Rename(path: string, newName: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/fs2/rename', {
    method: 'POST',
    body: JSON.stringify({ path, newName }),
  })
}

export function fs2Delete(paths: string[]): Promise<OpResult> {
  return api<OpResult>('/api/fs2/delete', {
    method: 'POST',
    body: JSON.stringify({ paths }),
  })
}

export function fetchPins(): Promise<{ pins: string[] }> {
  return api<{ pins: string[] }>('/api/fs2/pins')
}

export function addPin(path: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/fs2/pins', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

export function removePin(path: string): Promise<{ ok: boolean }> {
  const params = new URLSearchParams({ path })
  return api<{ ok: boolean }>(`/api/fs2/pins?${params.toString()}`, { method: 'DELETE' })
}
