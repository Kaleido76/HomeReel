import { api } from './client'
import type { Video } from './videos'
import type { Collection } from './collections'

export interface HomeData {
  continue_watching: Video[]
  recent: Video[]
  collections: Collection[]
}

export function fetchHome(): Promise<HomeData> {
  return api<HomeData>('/api/home')
}
