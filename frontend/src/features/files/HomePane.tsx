import { useEffect, useState } from 'react'
import { Folder, FileText, HardDrive, Pin, RefreshCw, Video } from 'lucide-react'
import type { Disk, MediaSource } from '../../api/files'
import { fetchFilesList } from '../../api/files'
import { formatBytes } from '../../lib/format'

export function HomePane({
  disks,
  pins,
  sources,
  onNavigate,
  onRescanSource,
}: {
  disks: Disk[]
  pins: string[]
  sources: MediaSource[]
  onNavigate: (path: string) => void
  onRescanSource: (path: string) => void
}) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex min-h-full w-full max-w-5xl justify-center px-6 py-16 lg:px-8">
        <div className="w-full space-y-12">
          <h1 className="text-xl font-medium text-neutral-800">文件库</h1>

          {disks.length > 0 && (
            <Section title="本地磁盘" icon={<HardDrive className="size-3.5" />}>
              <Grid>
                {disks.map((d) => {
                  const label = d.label ? `${d.path.replace(/\\$/, '')} · ${d.label}` : d.path
                  return (
                    <CoverCard
                      key={d.path}
                      title={label}
                      meta={d.total > 0 ? `${formatBytes(d.free)} 可用 / ${formatBytes(d.total)}` : undefined}
                      path={d.path}
                      onNavigate={onNavigate}
                    />
                  )
                })}
              </Grid>
            </Section>
          )}

          {pins.length > 0 && (
            <Section title="常用目录" icon={<Pin className="size-3.5" />}>
              <Grid>
                {pins.map((p) => (
                  <CoverCard key={p} title={p} path={p} onNavigate={onNavigate} />
                ))}
              </Grid>
            </Section>
          )}

          {sources.length > 0 && (
            <Section title="多媒体源" icon={<Video className="size-3.5" />}>
              <Grid>
                {sources.map((s) => {
                  const scanLabel = s.scanning
                    ? '扫描中…'
                    : s.last_scan_at
                      ? `上次扫描 ${new Date(s.last_scan_at).toLocaleDateString()}`
                      : '尚未扫描'
                  return (
                    <CoverCard
                      key={s.id}
                      title={s.path}
                      meta={!s.available ? `${scanLabel} · 离线` : scanLabel}
                      path={s.path}
                      onNavigate={onNavigate}
                      action={
                        <button
                          onClick={(e) => { e.stopPropagation(); onRescanSource(s.path) }}
                          disabled={s.scanning}
                          className="shrink-0 rounded p-1.5 text-neutral-400 opacity-0 transition-opacity hover:bg-neutral-100 hover:text-neutral-700 group-hover:opacity-100 disabled:opacity-30"
                          title="重新扫描"
                        >
                          <RefreshCw className="size-3.5" />
                        </button>
                      }
                    />
                  )
                })}
              </Grid>
            </Section>
          )}

          {disks.length === 0 && pins.length === 0 && sources.length === 0 && (
            <p className="py-16 text-center text-sm text-neutral-400">
              进入目录后可固定或标记为多媒体源
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

function Section({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <section>
      <h2 className="mb-4 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-neutral-400">
        {icon} {title}
      </h2>
      {children}
    </section>
  )
}

function Grid({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{children}</div>
}

// CoverCard is the unified card format for the home view: bold title, optional
// metadata line, optional right-side action, and a directory preview at the bottom.
function CoverCard({
  title,
  meta,
  path,
  onNavigate,
  action,
}: {
  title: string
  meta?: string
  path: string
  onNavigate: (p: string) => void
  action?: React.ReactNode
}) {
  return (
    <button
      onClick={() => onNavigate(path)}
      title={path}
      className="group flex w-full flex-col rounded-xl border border-neutral-200 bg-white text-left transition-colors select-none hover:bg-neutral-50"
    >
      <div className="flex items-start justify-between gap-2 px-5 py-4">
        <div className="min-w-0">
          <span className="block truncate text-sm font-medium">{title}</span>
          {meta && <span className="mt-0.5 block truncate text-xs text-neutral-400">{meta}</span>}
        </div>
        {action}
      </div>
      <DirectoryPreview path={path} />
    </button>
  )
}

function DirectoryPreview({ path }: { path: string }) {
  const [entries, setEntries] = useState<{ name: string; is_dir: boolean }[] | null>(null)

  useEffect(() => {
    let cancelled = false
    fetchFilesList(path)
      .then((res) => {
        if (cancelled) return
        setEntries(res.entries.slice(0, 6))
      })
      .catch(() => { if (!cancelled) setEntries([]) })
    return () => { cancelled = true }
  }, [path])

  if (entries === null) return null

  return (
    <div className="border-t border-neutral-100 px-5 py-2.5">
      {entries.length === 0 ? (
        <span className="block py-0.5 text-xs text-neutral-300">空目录</span>
      ) : (
        entries.map((e) => (
          <div key={e.name} className="flex items-center gap-1.5 py-0.5 text-xs text-neutral-400">
            {e.is_dir ? <Folder className="size-3 shrink-0" /> : <FileText className="size-3 shrink-0" />}
            <span className="min-w-0 truncate">{e.name}</span>
          </div>
        ))
      )}
    </div>
  )
}
