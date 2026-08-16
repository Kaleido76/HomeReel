import type { ReactNode } from 'react'
import { Eraser } from 'lucide-react'
import { Tooltip } from './Tooltip'

// HistorySection is one 小节 inside the 播放历史 card (see PlaybackHistoryCard):
// a header row (icon + title [+ note]) with an icon-only clear button whose label
// lives in a ToolTip, plus the section body. Both the 观看进度 and 播放选择记忆
// sections use it — the button's position (which section it sits in) together with
// the ToolTip text tells what it clears.
export function HistorySection({
  icon,
  title,
  note,
  clearTip,
  has,
  clearing,
  onClear,
  children,
}: {
  icon: ReactNode
  title: string
  note?: string
  clearTip: string
  has: boolean
  clearing?: boolean
  onClear: () => void
  children: ReactNode
}) {
  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-1.5 text-sm font-medium text-neutral-700">
          {icon}
          {title}
          {note ? <span className="text-xs font-normal text-neutral-400">{note}</span> : null}
        </div>
        <Tooltip content={clearTip}>
          <button
            onClick={onClear}
            disabled={clearing || !has}
            className="flex items-center gap-1 rounded border border-neutral-200 px-2 py-1 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-900 disabled:opacity-40"
          >
            <Eraser className="size-3.5" />
          </button>
        </Tooltip>
      </div>
      {children}
    </section>
  )
}
