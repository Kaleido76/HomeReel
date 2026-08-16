import type { ReactNode } from 'react'

// PlaybackHistoryCard is the merged 播放历史 card of the series / single-episode
// detail pages: it groups the 观看进度 section and the 配置缓存 section under one
// titled card, each section being a self-contained HistorySection provided by the
// caller. The two pages keep their own queries/mutations while sharing this shell.
export function PlaybackHistoryCard({ progress, memory }: { progress: ReactNode; memory: ReactNode }) {
  return (
    <div className="rounded-md border border-neutral-200 bg-white p-4">
      <p className="text-sm font-medium text-neutral-900">播放历史</p>
      <div className="mt-3">{progress}</div>
      <div className="mt-4 border-t border-neutral-100 pt-4">{memory}</div>
    </div>
  )
}
