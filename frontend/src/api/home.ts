import { api } from './client'
import type { Video } from './videos'

export interface HomeData {
  continue_watching: Video[]
  recent: Video[]
}

export function fetchHome(): Promise<HomeData> {
  return api<HomeData>('/api/home')
}
