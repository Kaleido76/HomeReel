import { useState } from 'react'
import { CheckCircle2, RefreshCw, XCircle } from 'lucide-react'
import type { Job } from '../../api/jobs'
import { isActiveJob, isNotableJob } from '../../api/jobs'
import { formatEta } from '../../lib/format'
import { Tooltip } from '../../components/Tooltip'
import { useJobs } from './useJobs'

// JobsIndicator is the JetBrains-style task status button in the header. The
// double-arrow icon spins while any long task is running; clicking toggles a
// small panel that lists running/queued tasks with their progress. The panel
// only covers its own bounds and never dismisses on outside clicks — the same
// button is the only toggle.
export function JobsIndicator() {
  const [open, setOpen] = useState(false)
  const jobs = useJobs()
  const notable = (jobs.data?.jobs ?? []).filter(isNotableJob)
  const active = notable.filter(isActiveJob)
  const recent = notable.filter((j) => !isActiveJob(j)).slice(0, 5)
  const hasActive = active.length > 0

  return (
    <div className="relative">
      <Tooltip content={hasActive ? `${active.length} 个后台任务进行中` : '后台任务'}>
        <button
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-haspopup="dialog"
          className="relative flex shrink-0 items-center rounded-md px-2.5 py-1.5 text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
        >
          <RefreshCw className={`size-4 ${hasActive ? 'animate-spin text-blue-600' : ''}`} />
          {hasActive && (
            <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-600 px-1 text-[10px] font-medium leading-none text-white">
              {active.length}
            </span>
          )}
        </button>
      </Tooltip>

      {open && (
        <div
          role="dialog"
          aria-label="后台任务"
          className="absolute right-0 top-full z-50 mt-1 w-80 overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-lg"
        >
          <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2">
            <p className="text-sm font-medium text-neutral-900">后台任务</p>
            <span className="text-xs text-neutral-400">
              {active.length > 0 ? `${active.length} 个进行中` : '全部完成'}
            </span>
          </div>
          <div className="max-h-96 overflow-y-auto">
            {active.length === 0 && recent.length === 0 ? (
              <p className="px-3 py-8 text-center text-sm text-neutral-400">暂无任务</p>
            ) : (
              <>
                {active.map((j) => (
                  <JobRow key={j.id} job={j} />
                ))}
                {recent.length > 0 && (
                  <p className="sticky top-0 border-t border-neutral-100 bg-neutral-50 px-3 py-1.5 text-xs font-medium text-neutral-400">
                    最近
                  </p>
                )}
                {recent.map((j) => (
                  <JobRow key={j.id} job={j} />
                ))}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function JobRow({ job }: { job: Job }) {
  const active = isActiveJob(job)
  const determinate = job.progress >= 0
  const pct = Math.round(job.progress * 100)
  const hasSubtask = active && !!job.subtask
  const subtaskPct = (job.subtask_progress ?? -1) >= 0 ? Math.round(job.subtask_progress ?? 0) : null
  const eta = job.eta_seconds != null ? formatEta(job.eta_seconds) : ''

  return (
    <div className="flex items-start gap-2 px-3 py-2">
      {job.status === 'failed' ? (
        <XCircle className="mt-0.5 size-4 shrink-0 text-red-500" />
      ) : job.status === 'done' ? (
        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600" />
      ) : (
        <RefreshCw className={`mt-0.5 size-4 shrink-0 ${active ? 'animate-spin text-blue-600' : 'text-neutral-400'}`} />
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-neutral-800">{job.name || job.type}</p>
        {active && (
          <div className="mt-1 h-1 overflow-hidden rounded-sm bg-neutral-200">
            {determinate ? (
              <div className="h-full bg-blue-600 transition-all" style={{ width: `${pct}%` }} />
            ) : (
              <div className="indeterminate-bar h-full w-1/2 bg-blue-600" />
            )}
          </div>
        )}
        {active && determinate && job.status === 'running' && (
          <p className="mt-1 text-xs text-neutral-400">
            {pct}%
            {eta ? ` · 预计还需 ${eta}` : ''}
          </p>
        )}
        {active && !determinate && (
          <p className="mt-1 text-xs text-neutral-400">{job.status === 'queued' ? '排队中…' : '处理中…'}</p>
        )}
        {hasSubtask && (
          <p className="mt-1 flex items-center gap-1.5 truncate text-xs text-neutral-500">
            <span className="min-w-0 truncate">{job.subtask}</span>
            {subtaskPct !== null && <span className="shrink-0 text-neutral-400">{subtaskPct}%</span>}
          </p>
        )}
        {job.status === 'failed' && <p className="mt-1 truncate text-xs text-red-500">{job.error}</p>}
        {job.status === 'done' && <p className="mt-1 text-xs text-neutral-400">已完成</p>}
      </div>
    </div>
  )
}
