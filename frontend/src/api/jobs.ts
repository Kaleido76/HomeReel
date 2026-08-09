import { api } from './client'

export type JobStatus = 'queued' | 'running' | 'done' | 'failed'

export interface Job {
  id: string
  type: string
  name: string
  target: string
  extra: string
  status: JobStatus
  progress: number
  error: string
  internal: boolean
  created_at: string
  updated_at: string
  subtask?: string
  subtask_progress?: number
  // eta_seconds is the backend's live estimate of remaining work (only present
  // while the job reports determinate progress).
  eta_seconds?: number
}

// A job is "active" while it is still owned by the worker; these are the rows
// the task panel shows as in progress.
export function isActiveJob(j: Job): boolean {
  return j.status === 'queued' || j.status === 'running'
}

// A "notable" job is a user-facing long task (scan / convert / future tasks).
// Internal maintenance jobs (probe/thumbnail) are hidden from the task panel
// so a scan's probe burst never floods it.
export function isNotableJob(j: Job): boolean {
  return !j.internal
}

export function hasActiveJobs(jobs: Job[] | undefined): boolean {
  return (jobs ?? []).some((j) => isActiveJob(j) && isNotableJob(j))
}

export function fetchJobs(limit = 50): Promise<{ jobs: Job[] }> {
  return api<{ jobs: Job[] }>(`/api/jobs?limit=${limit}`)
}
