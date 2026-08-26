import { useEffect } from 'react'
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { fetchJobs } from '../../api/jobs'
import { useRealtime, useRealtimeMessage } from '../../components/RealtimeProvider'
import type { Job } from '../../api/jobs'

export const jobsKey = ['jobs'] as const

// useJobs keeps the task-panel job list live over the realtime channel instead
// of polling (ADR-021). The backend publishes every job change as a
// "jobs.progress" push (enqueue → running progress → final done/failed), and
// the frontend merges it into the cache in place, so no REST refetch happens
// during normal operation. The only REST call is the initial snapshot on mount;
// a single refetch also fires when the realtime connection (re)establishes, to
// recover anything missed while disconnected.
export function useJobs() {
  const queryClient = useQueryClient()
  const { status } = useRealtime()
  const query = useQuery({
    queryKey: jobsKey,
    queryFn: () => fetchJobs(50),
    // No polling: the WebSocket keeps the cache fresh. refetchOnWindowFocus is
    // disabled so merely focusing the window does not hit /api/jobs.
    refetchOnWindowFocus: false,
  })

  // Merge every job snapshot into the cache (smooth bars, no refetch).
  useRealtimeMessage('jobs.progress', (data) => mergeJob(queryClient, data as Job))

  // On (re)connect, pull a fresh snapshot once to recover missed updates.
  useEffect(() => {
    if (status === 'connected') {
      void queryClient.invalidateQueries({ queryKey: jobsKey })
    }
  }, [status, queryClient])

  return query
}

// mergeJob writes a single job into the jobs list cache. New jobs are prepended
// (the list is newest-first); existing jobs are replaced in place by id.
function mergeJob(queryClient: QueryClient, job: Job) {
  queryClient.setQueryData<{ jobs: Job[] }>(jobsKey, (old) => {
    if (!old) return old
    const jobs = old.jobs
    const idx = jobs.findIndex((j) => j.id === job.id)
    if (idx < 0) {
      return { jobs: [job, ...jobs] }
    }
    const next = jobs.slice()
    next[idx] = job
    return { jobs: next }
  })
}
