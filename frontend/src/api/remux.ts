import { api } from './client'

// RemuxItem is one segmented MP4 (hls.js-assembled) and whether a remuxed
// faststart copy already exists for it.
export interface RemuxItem {
  video_id: string
  title: string
  relative_path: string
  storage_id: string
  segmented: boolean
  remuxed: boolean
}

export function fetchRemuxStatus(): Promise<{ items: RemuxItem[] }> {
  return api<{ items: RemuxItem[] }>('/api/remux/status')
}

// requestRemux schedules a remux job for one video.
export function requestRemux(id: string): Promise<{ accepted: boolean }> {
  return api<{ accepted: boolean }>(`/api/videos/${id}/remux`, { method: 'POST' })
}

// requestFolderRemux schedules remux jobs for every segmented video under a
// storage folder.
export function requestFolderRemux(storageId: string, path: string): Promise<{ accepted: number }> {
  return api<{ accepted: number }>('/api/fs/remux', {
    method: 'POST',
    body: JSON.stringify({ storageId, path }),
  })
}
