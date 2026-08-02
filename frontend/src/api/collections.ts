import { api } from './client'
import type { Video } from './videos'

export interface Collection {
  id: string
  name: string
  created_at: string
}

export function fetchCollections(): Promise<{ collections: Collection[] }> {
  return api<{ collections: Collection[] }>('/api/collections')
}

export function createCollection(name: string): Promise<{ collection: Collection }> {
  return api<{ collection: Collection }>('/api/collections', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function renameCollection(id: string, name: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/collections/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  })
}

export function deleteCollection(id: string): Promise<{ deleted: boolean }> {
  return api<{ deleted: boolean }>(`/api/collections/${id}`, { method: 'DELETE' })
}

export function fetchCollectionVideos(id: string): Promise<{ videos: Video[] }> {
  return api<{ videos: Video[] }>(`/api/collections/${id}/videos`)
}

export function addToCollection(collectionId: string, videoId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/collections/${collectionId}/videos/${videoId}`, { method: 'PUT' })
}

export function removeFromCollection(collectionId: string, videoId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/collections/${collectionId}/videos/${videoId}`, { method: 'DELETE' })
}
