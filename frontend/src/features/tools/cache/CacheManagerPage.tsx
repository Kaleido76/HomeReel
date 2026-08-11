import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Eraser, FolderOpen, Images, Subtitles, Trash2 } from 'lucide-react'
import {
  clearAllSubtitleCache,
  clearOrphanCache,
  clearSubtitleCache,
  clearSubtitleTrack,
  fetchCacheStats,
  type CacheOrphans,
  type SubtitleCacheGroup,
} from '../../../api/cache'
import { formatBytes } from '../../../lib/format'

// CacheManagerPage manages the regenerable caches around videos. Granularity
// follows what is actually useful: extracted subtitles are listed per video
// (grouped under their show) and can be cleared per track / per video, since
// they are the most frequently stale; covers and thumbs are only ever cleared
// as orphans — an in-use cover/thumb is regenerated at scan time and deleting
// it has no upside. Clearing never touches source files.
export function CacheManagerPage() {
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const overview = useQuery({ queryKey: ['cache'], queryFn: fetchCacheStats })

  const orphans = overview.data?.orphans
  const subtitleGroups = overview.data?.subtitles ?? []
  const orphanTotal = orphans ? orphanCount(orphans) : 0

  async function run(action: () => Promise<{ cleared: number }>, confirmText: string) {
    if (!window.confirm(confirmText)) return
    setError('')
    setMessage('')
    try {
      const { cleared } = await action()
      await queryClient.invalidateQueries({ queryKey: ['cache'] })
      setMessage(`已清理 ${cleared} 个缓存文件`)
    } catch (err) {
      setError(err instanceof Error ? err.message : '清理失败')
    }
  }

  // Group live subtitles under their show (in backend order); standalone videos
  // are listed separately.
  const showGroups: { title: string; groups: SubtitleCacheGroup[] }[] = []
  const standalone: SubtitleCacheGroup[] = []
  const showIndex = new Map<string, number>()
  for (const g of subtitleGroups) {
    if (g.show_id && g.show_title) {
      let idx = showIndex.get(g.show_id)
      if (idx === undefined) {
        idx = showGroups.length
        showIndex.set(g.show_id, idx)
        showGroups.push({ title: g.show_title, groups: [] })
      }
      showGroups[idx].groups.push(g)
    } else {
      standalone.push(g)
    }
  }

  return (
    <div className="mx-auto max-w-3xl">
      <p className="text-sm text-neutral-600">
        这里管理围绕视频生成的缓存：封面/缩略图在扫描时生成，字幕 vtt 在内封字幕首次播放时提取。清理不影响源文件，缓存均可重建。
      </p>

      <section className="mt-6 rounded-md border border-neutral-200 bg-white p-4">
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0">
            <h2 className="text-sm font-medium text-neutral-800">孤儿缓存</h2>
            <p className="mt-1 text-xs text-neutral-500">
              对应视频已不在库中的残留文件（删除记录后可能遗留），只能整体清理。
            </p>
            {orphans && orphanTotal > 0 ? (
              <p className="mt-2 text-sm text-neutral-800">
                {orphanTotal} 个 · {formatBytes(orphanBytes(orphans))}
                <span className="ml-2 text-xs text-neutral-400">
                  封面 {orphans.cover.orphans} · 缩略图 {orphans.thumb.orphans} · 字幕 {orphans.subtitle.orphans}
                </span>
              </p>
            ) : (
              <p className="mt-2 text-sm text-emerald-600">当前没有孤儿缓存</p>
            )}
          </div>
          <button
            disabled={!orphans || orphanTotal === 0}
            onClick={() => void run(clearOrphanCache, `确定清理全部 ${orphanTotal} 个孤儿缓存文件？`)}
            className="flex shrink-0 items-center gap-1.5 rounded bg-neutral-900 px-4 py-2 text-sm text-white hover:bg-neutral-800 disabled:cursor-not-allowed disabled:bg-neutral-300"
          >
            <Eraser className="size-4" /> 清理孤儿
          </button>
        </div>
      </section>

      <section className="mt-6 rounded-md border border-neutral-200 bg-white p-4">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-1.5">
            <Subtitles className="size-4 text-neutral-400" />
            <h2 className="text-sm font-medium text-neutral-800">字幕缓存</h2>
          </div>
          <button
            onClick={() => void run(clearAllSubtitleCache, '确定清空全部字幕缓存？下次播放会自动重新提取。')}
            className="flex items-center gap-1.5 rounded border border-neutral-300 px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-100"
          >
            <Eraser className="size-3.5" /> 清空全部
          </button>
        </div>
        <p className="mt-1 text-xs text-neutral-500">
          内封文本字幕首次播放时提取并缓存；源文件更换或提取乱码时可逐条删除，重播即重新生成。
        </p>

        {subtitleGroups.length === 0 ? (
          <p className="mt-4 text-sm text-neutral-500">
            尚无字幕缓存。播放过含内封字幕的视频（如 MKV）后，这里会列出其提取出的字幕文件。
          </p>
        ) : (
          <div className="mt-4 space-y-6">
            {showGroups.map((sg) => (
              <div key={sg.title}>
                <h3 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                  <FolderOpen className="size-3.5" /> {sg.title}
                </h3>
                <div className="mt-2 space-y-3">
                  {sg.groups.map((g) => (
                    <SubtitleVideoRow key={g.video_id} group={g} run={run} />
                  ))}
                </div>
              </div>
            ))}
            {standalone.length > 0 && (
              <div>
                <h3 className="text-xs font-semibold uppercase tracking-wide text-neutral-500">未归组的视频</h3>
                <div className="mt-2 space-y-3">
                  {standalone.map((g) => (
                    <SubtitleVideoRow key={g.video_id} group={g} run={run} />
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </section>

      <section className="mt-6 rounded-md border border-neutral-200 bg-white p-4">
        <div className="flex items-center gap-1.5">
          <Images className="size-4 text-neutral-400" />
          <h2 className="text-sm font-medium text-neutral-800">封面与缩略图</h2>
        </div>
        <p className="mt-1 text-xs text-neutral-500">
          扫描时自动生成。正在使用的封面/缩略图无需删除；如视频已删除但残留了图片，请用上方的「清理孤儿」。
        </p>
        {orphans && (orphans.cover.orphans > 0 || orphans.thumb.orphans > 0) && (
          <p className="mt-2 text-sm text-neutral-800">
            当前有孤儿封面 {orphans.cover.orphans} 个、孤儿缩略图 {orphans.thumb.orphans} 个。
          </p>
        )}
      </section>

      {message && <p className="mt-4 text-sm text-emerald-600">{message}</p>}
      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}
    </div>
  )
}

function orphanCount(o: CacheOrphans): number {
  return o.cover.orphans + o.thumb.orphans + o.subtitle.orphans
}

function orphanBytes(o: CacheOrphans): number {
  return o.cover.orphan_bytes + o.thumb.orphan_bytes + o.subtitle.orphan_bytes
}

function SubtitleVideoRow({
  group,
  run,
}: {
  group: SubtitleCacheGroup
  run: (action: () => Promise<{ cleared: number }>, confirmText: string) => void
}) {
  const [busy, setBusy] = useState(false)
  return (
    <div className="rounded border border-neutral-200 p-3">
      <div className="flex items-center justify-between gap-4">
        <span className="min-w-0 truncate text-sm font-medium text-neutral-800">{group.title}</span>
        <span className="shrink-0 text-xs text-neutral-400">
          {group.files.length} 轨 · {formatBytes(group.bytes)}
        </span>
        <button
          disabled={busy}
          onClick={() =>
            void (async () => {
              setBusy(true)
              await run(
                () => clearSubtitleCache(group.video_id),
                `确定清空「${group.title}」的字幕缓存（${group.files.length} 个文件）？`,
              )
              setBusy(false)
            })()
          }
          className="flex shrink-0 items-center gap-1 rounded border border-neutral-300 px-2 py-1 text-xs text-neutral-700 hover:bg-neutral-100 disabled:opacity-50"
        >
          <Trash2 className="size-3" /> 清空该视频
        </button>
      </div>
      <ul className="mt-2 space-y-1">
        {group.files.map((f) => (
          <li key={f.name} className="flex items-center justify-between gap-4 py-0.5 pl-4">
            <span className="min-w-0 truncate text-xs text-neutral-600">
              轨 {f.track} · {f.name} · {formatBytes(f.bytes)}
            </span>
            <button
              disabled={busy}
              onClick={() =>
                void (async () => {
                  setBusy(true)
                  await run(() => clearSubtitleTrack(group.video_id, f.track), `删除字幕文件 ${f.name}？`)
                  setBusy(false)
                })()
              }
              className="flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs text-red-600 hover:bg-red-50 disabled:opacity-50"
            >
              <Trash2 className="size-3" /> 删除
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
