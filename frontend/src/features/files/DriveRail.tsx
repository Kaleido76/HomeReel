import { useRef } from 'react'
import type { MouseEvent as ReactMouseEvent } from 'react'
import { HardDrive, Loader2, Pin, RefreshCw, Usb, Video } from 'lucide-react'
import type { Disk } from '../../api/files'
import type { MediaSource } from '../../api/files'
import { underDrive } from './path'
import { Tooltip } from '../../components/Tooltip'
import { useRailSplit } from './railSplit'

const TOP_SPLIT_KEY = 'files.railSplit'
const BOTTOM_SPLIT_KEY = 'files.railSplitBottom'
const LEGACY_TOP_SPLIT_KEY = 'filesnew.railSplit'
const LEGACY_BOTTOM_SPLIT_KEY = 'filesnew.railSplitBottom'

// DriveRail is the full-height narrow left frame of the file browser, split by
// two independent draggable rules: 盘符 (top) vs 常用+多媒体源 (bottom block), and
// within that block 常用 vs 多媒体源. Each divider controls only its own
// boundary, so dragging one never nudges the other.
export function DriveRail({
  disks,
  pins,
  sources,
  currentPath,
  onNavigate,
  onRescanSource,
}: {
  disks: Disk[]
  pins: string[]
  sources: MediaSource[]
  currentPath: string
  onNavigate: (path: string) => void
  onRescanSource: (path: string) => void
}) {
  const railRef = useRef<HTMLElement | null>(null)
  const blockRef = useRef<HTMLDivElement | null>(null)
  const top = useRailSplit(TOP_SPLIT_KEY, LEGACY_TOP_SPLIT_KEY, 0.45, railRef)
  const bottom = useRailSplit(BOTTOM_SPLIT_KEY, LEGACY_BOTTOM_SPLIT_KEY, 0.5, blockRef)

  return (
    <aside ref={railRef} className="flex h-full w-56 shrink-0 flex-col border-r border-neutral-200 bg-white">
      <section style={{ height: `${top.ratio * 100}%` }} className="flex min-h-0 flex-col">
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

      <RailDivider onMouseDown={top.startDrag} />

      <div ref={blockRef} className="flex min-h-0 flex-1 flex-col">
        <section style={{ flexBasis: `${bottom.ratio * 100}%` }} className="flex min-h-0 flex-col">
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

        <RailDivider onMouseDown={bottom.startDrag} />

        <section className="flex min-h-0 flex-1 flex-col border-t border-neutral-100">
          <p className="flex items-center gap-1.5 px-3 pb-1 pt-2.5 text-xs font-medium uppercase tracking-wide text-neutral-400">
            <Video className="size-3.5" /> 多媒体源
          </p>
          <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
            {sources.length === 0 ? (
              <p className="px-2 py-3 text-sm leading-relaxed text-neutral-400">
                可在工具栏把当前目录标记为多媒体源，其下的视频会自动入库
              </p>
            ) : (
              sources.map((s) => {
                const active = currentPath === s.path
                return (
                  <div
                    key={s.id}
                    className={`group flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors ${
                      active ? 'bg-blue-50' : ''
                    }`}
                  >
                    <button
                      onClick={() => onNavigate(s.path)}
                      title={s.path}
                      className={`flex min-w-0 flex-1 items-center gap-1.5 text-left ${active ? 'text-blue-700' : 'text-neutral-700 hover:text-neutral-900'}`}
                    >
                      <span className="min-w-0 flex-1 truncate">{s.path}</span>
                      {s.scanning && <Loader2 className="size-3 shrink-0 animate-spin text-blue-500" />}
                      {!s.available && <span className="shrink-0 rounded bg-neutral-200 px-1 text-[10px] text-neutral-500">离线</span>}
                    </button>
                    <Tooltip content="重新扫描">
                      <button
                        onClick={() => onRescanSource(s.path)}
                        disabled={s.scanning}
                        className="shrink-0 rounded p-1 text-neutral-400 opacity-0 transition-opacity hover:bg-neutral-100 hover:text-neutral-700 group-hover:opacity-100 disabled:opacity-30"
                      >
                        <RefreshCw className="size-3.5" />
                      </button>
                    </Tooltip>
                  </div>
                )
              })
            )}
          </div>
        </section>
      </div>
    </aside>
  )
}

function RailDivider({ onMouseDown }: { onMouseDown: (e: ReactMouseEvent) => void }) {
  return (
    <div
      onMouseDown={onMouseDown}
      title="拖动调整分栏"
      className="group flex h-2 shrink-0 cursor-row-resize items-center justify-center bg-neutral-100 transition-colors hover:bg-neutral-200"
    >
      <span className="h-0.5 w-12 rounded-full bg-neutral-300 transition-colors group-hover:bg-neutral-500" />
    </div>
  )
}
