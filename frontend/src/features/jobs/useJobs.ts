import { useQuery } from '@tanstack/react-query'
import { fetchJobs, hasActiveJobs } from '../../api/jobs'

export const jobsKey = ['jobs'] as const

// useJobs polls the job queue; while a task is active it refreshes every
// second so the header indicator and progress bars stay live, and backs off
// once idle.
export function useJobs() {
  return useQuery({
    queryKey: jobsKey,
    queryFn: () => fetchJobs(50),
    refetchInterval: (query) => (hasActiveJobs(query.state.data?.jobs) ? 1000 : 15_000),
  })
}
