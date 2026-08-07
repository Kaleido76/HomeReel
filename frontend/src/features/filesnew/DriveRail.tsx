import { useRef, useState } from 'react'
import { HardDrive, Pin, Usb } from 'lucide-react'
import type { Disk } from '../../api/fsbrowse'
import { useWindowDrag } from './drag'
import { underDrive } from './path'

const RAIL_SPLIT_KEY = 'filesnew.railSplit'
const MIN_SPLIT = 0.15
const MAX_SPLIT = 0.85

function loadSplit(): number {
  try {
    const raw = localStorage.getItem(RAIL_SPLIT_KEY)
    if (raw) {
      const n = Number(raw)
      if (Number.isFinite(n)) return Math.min(MAX_SPLIT, Math.max(MIN_SPLIT, n))
    }
  } catch {
    // fall through to default
  }
  return 0.5
}

// DriveRail is the full-height narrow left frame of the file browser. It is
// split into two sections by a draggable horizontal rule: the top lists every
// local drive on the host (auto-enumerated, nothing to configure), the bottom is
// the pinned paths panel. Dragging the divider resizes the two sections and the
// ratio persists in localStorage. Future sections (bookmarks, filters…) can
// stack below the pins panel.
export function DriveRail({
  disks,
  pins,
  currentPath,
  onNavigate,
}: {
  disks: Disk[]
  pins: string[]
  currentPath: string
  onNavigate: (path: string) => void
}) {
  const [split, setSplit] = useState(loadSplit)
  const railRef = useRef<HTMLElement>(null)
  const dragSplit = useRef<{ startSplit: number } | null>(null)

  const beginDrag = useWindowDrag((_dx, dy) => {
    if (!dragSplit.current || !railRef.current) return
    const { startSplit } = dragSplit.current
    const h = railRef.current.getBoundingClientRect().height
    if (h <= 0) return
    const next = Math.min(MAX_SPLIT, Math.max(MIN_SPLIT, startSplit + dy / h))
    setSplit(next)
    try {
      localStorage.setItem(RAIL_SPLIT_KEY, String(next))
    } catch {
      // ignore quota errors
    }
  })

  return (
    <aside ref={railRef} className="flex h-full w-56 shrink-0 flex-col border-r border-neutral-200 bg-white">
      <section style={{ flexBasis: `${split * 100}%` }} className="flex min-h-0 flex-col">
        <p className="flex items-center gap-1.5 px-3 pb-1 pt-2.5 text-xs font-medium uppercase tracking-wide text-neutral-400">
          <HardDrive className="size-3.5" /> 盘符
        </p>
        <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
          {disks.length === 0 ? (
            <p className="px-2 py-3 text-sm text-neutral-400">未检测到本地磁盘</p>
          ) : (
            disks.map((d) => {
              const active = underDrive(currentPath, d.path)
              const Icon = d.type === 'removable' ? Usb : HardDrive
              const label = d.label ? `${d.path.replace(/\\$/, '')} · ${d.label}` : d.path
              return (
                <button
                  key={d.path}
                  onClick={() => onNavigate(d.path)}
                  title={label}
                  className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors ${
                    active ? 'bg-blue-50 text-blue-700' : 'text-neutral-700 hover:bg-neutral-100'
                  }`}
                >
                  <Icon className="size-4 shrink-0 text-neutral-400" />
                  <span className="min-w-0 flex-1 truncate">{label}</span>
                </button>
              )
            })
          )}
        </div>
      </section>

      <div
        onMouseDown={(e) => {
          dragSplit.current = { startSplit: split }
          beginDrag(e)
        }}
        title="拖动调整分栏"
        className="group flex h-2 shrink-0 cursor-row-resize items-center justify-center bg-neutral-100 transition-colors hover:bg-neutral-200"
      >
        <span className="h-0.5 w-12 rounded-full bg-neutral-300 transition-colors group-hover:bg-neutral-500" />
      </div>

      <section className="flex min-h-0 flex-1 flex-col">
        <p className="flex items-center gap-1.5 px-3 pb-1 pt-2.5 text-xs font-medium uppercase tracking-wide text-neutral-400">
          <Pin className="size-3.5" /> 常用
        </p>
        <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
          {pins.length === 0 ? (
            <p className="px-2 py-3 text-sm text-neutral-400">暂无固定路径，可在工具栏固定当前目录</p>
          ) : (
            pins.map((p) => {
              const active = currentPath === p
              return (
                <button
                  key={p}
                  onClick={() => onNavigate(p)}
                  title={p}
                  className={`flex w-full items-center rounded-lg px-2 py-1.5 text-left text-sm transition-colors ${
                    active ? 'bg-blue-50 text-blue-700' : 'text-neutral-700 hover:bg-neutral-100'
                  }`}
                >
                  <span className="min-w-0 flex-1 truncate">{p}</span>
                </button>
              )
            })
          )}
        </div>
      </section>
    </aside>
  )
}
