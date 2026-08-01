import { api } from './client'

export type StorageType = 'internal' | 'external' | 'network'

export interface Storage {
  id: string
  name: string
  type: StorageType
  root_path: string
  device_id?: string
  readonly: boolean
  enabled: boolean
  available: boolean
  created_at: string
}

export interface FsEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mtime: number
  is_video: boolean
}

export interface FsListResult {
  storage: Storage
  path: string
  entries: FsEntry[]
}

export interface StorageInput {
  name: string
  type: StorageType
  root_path: string
  device_id?: string
  readonly?: boolean
  enabled?: boolean
}

export function fetchStorages(): Promise<{ storages: Storage[] }> {
  return api<{ storages: Storage[] }>('/api/storages')
}

export function createStorage(input: StorageInput): Promise<{ storage: Storage }> {
  return api<{ storage: Storage }>('/api/storages', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function patchStorage(id: string, input: Partial<StorageInput>): Promise<{ storage: Storage }> {
  return api<{ storage: Storage }>(`/api/storages/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(input),
  })
}

export function deleteStorage(id: string): Promise<{ deleted: boolean }> {
  return api<{ deleted: boolean }>(`/api/storages/${id}`, { method: 'DELETE' })
}

export function refreshStorage(id: string): Promise<{ storage: Storage }> {
  return api<{ storage: Storage }>(`/api/storages/${id}/refresh`, { method: 'POST' })
}

export function fetchFsList(storageId: string, path: string): Promise<FsListResult> {
  const params = new URLSearchParams({ storageId })
  if (path) params.set('path', path)
  return api<FsListResult>(`/api/fs/list?${params.toString()}`)
}

export interface OpResult {
  done: number
  errors?: { path: string; message: string }[]
}

export function fsMkdir(storageId: string, path: string, name: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/fs/mkdir', {
    method: 'POST',
    body: JSON.stringify({ storageId, path, name }),
  })
}

export function fsRename(storageId: string, path: string, newName: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>('/api/fs/rename', {
    method: 'POST',
    body: JSON.stringify({ storageId, path, newName }),
  })
}

export function fsMove(storageId: string, paths: string[], dest: string): Promise<OpResult> {
  return api<OpResult>('/api/fs/move', {
    method: 'POST',
    body: JSON.stringify({ storageId, paths, dest }),
  })
}

export function fsDelete(storageId: string, paths: string[]): Promise<OpResult> {
  return api<OpResult>('/api/fs/delete', {
    method: 'POST',
    body: JSON.stringify({ storageId, paths }),
  })
}

export function fsDownloadUrl(storageId: string, path: string): string {
  return `/api/fs/download?storageId=${encodeURIComponent(storageId)}&path=${encodeURIComponent(path)}`
}

export function uploadChunk(
  storageId: string,
  path: string,
  uploadId: string,
  filename: string,
  chunkIndex: number,
  chunkTotal: number,
  blob: Blob,
): Promise<{ received: number; complete: boolean }> {
  const form = new FormData()
  form.append('uploadId', uploadId)
  form.append('filename', filename)
  form.append('chunkIndex', String(chunkIndex))
  form.append('chunkTotal', String(chunkTotal))
  form.append('file', blob, filename)
  return api<{ received: number; complete: boolean }>(
    `/api/upload?storageId=${encodeURIComponent(storageId)}&path=${encodeURIComponent(path)}`,
    { method: 'POST', body: form, signal: AbortSignal.timeout(120_000) },
  )
}
