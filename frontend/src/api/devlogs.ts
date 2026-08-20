import { api } from './client'
import type { DevLogLevel } from '../lib/devlog'

export interface DevLogEntry {
  timestamp: string
  level: DevLogLevel
  module: string
  message: string
}

export interface DevLogSummary {
  id: string
  source: string
  note: string
  count: number
  created_at: string
}

export interface DevLogArchive {
  id: string
  source: string
  note: string
  entries: DevLogEntry[]
  created_at: string
}

export function submitDevLog(source: string, note: string, entries: DevLogEntry[]): Promise<{ id: string }> {
  return api<{ id: string }>('/api/devlogs', {
    method: 'POST',
    body: JSON.stringify({ source, note, entries }),
  })
}

export function fetchDevLogs(): Promise<{ items: DevLogSummary[] }> {
  return api<{ items: DevLogSummary[] }>('/api/devlogs')
}

export function fetchDevLog(id: string): Promise<DevLogArchive> {
  return api<DevLogArchive>(`/api/devlogs/${id}`)
}

export function deleteDevLog(id: string): Promise<{ deleted: boolean }> {
  return api<{ deleted: boolean }>(`/api/devlogs/${id}`, { method: 'DELETE' })
}
