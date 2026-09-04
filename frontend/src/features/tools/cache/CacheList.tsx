import { useState, type ReactNode } from 'react'
import { Eraser, FileX, Layers, Search } from 'lucide-react'
import { clearOrphanCache } from '../../../api/cache'
import type { CacheOrphans } from '../../../api/cache'
import { seriesPosterUrl } from '../../../api/series'
import { coverUrl } from '../../../api/videos'
import { useNotify } from '../../../components/NotificationProvider'
import { formatBytes } from '../../../lib/format'
import {
  hasCache,
  hasStandaloneCache,
  orphanBytesTotal,
  orphanTotal,
  type CacheSelection,
  type SeriesCacheInfo,
  type StandaloneCacheInfo,
} from './model'
import { ConfirmDialog } from './ConfirmDialog'

// CacheList is the left navigation pane: a search box, the pinned orphan cache
// item, the cached series and standalone videos — each with a one-line
// 「显示无缓存的…」toggle for its grayed no-cache rows (library 单集 semantics).
export function CacheList({
  series,
  standalone,
  noCacheCount,
  showAll,
  onToggleShowAll,
  standaloneNoCacheCount,
  standaloneShowAll,
  onToggleStandaloneShowAll,
  query,
  onQuery,
  selection,
  onSelect,
  orphans,
  onChanged,
}: {
  series: SeriesCacheInfo[]
  standalone: StandaloneCacheInfo[]
  noCacheCount: number
  showAll: boolean
  onToggleShowAll: () => void
  standaloneNoCacheCount: number
  standaloneShowAll: boolean
  onToggleStandaloneShowAll: () => void
  query: string
  onQuery: (q: string) => void
  selection: CacheSelection | null
  onSelect: (sel: CacheSelection) => void
  orphans?: CacheOrphans
  onChanged: () => Promise<void>
}) {
  const [confirmOrphans, setConfirmOrphans] = useState(false)
  const { notify } = useNotify()
  const orphanCount = orphans ? orphanTotal(orphans) : 0
  const empty = series.length === 0 && standalone.length === 0

  return (
    <div className="flex min-h-0 flex-col rounded-lg border border-neutral-200 bg-white">
      {/* Search */}
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-100 px-3 py-2.5">
        <Search className="size-4 shrink-0 text-neutral-400" />
        <input
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder="搜索系列或单集标题…"
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-neutral-400"
        />
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {/* Pinned global cache: orphan only (remux is cleared within its owning
            series/video; unowned remux counts as orphan) */}
        <ul className="space-y-1 px-2 pb-2 pt-2">
          {orphanCount > 0 && (
            <li>
              <GlobalRow
                icon={<FileX className="size-4" />}
                title="孤儿缓存"
                sub={`${orphanCount} 个 · ${formatBytes(orphanBytesTotal(orphans!))}（封面/缩略图/字幕/Remux 残留）`}
                onAction={() => setConfirmOrphans(true)}
              />
            </li>
          )}
        </ul>

        {series.length > 0 && (
          <p className="px-3 pb-1 pt-3 text-xs font-medium uppercase tracking-wide text-neutral-400">
            系列缓存（{series.length}）
          </p>
        )}
        <ul className="space-y-1 px-2">
          {series.map((s) => (
            <SeriesRow
              key={s.series.id}
              info={s}
              active={selection?.kind === 'series' && selection.id === s.series.id}
              onSelect={() => onSelect({ kind: 'series', id: s.series.id })}
            />
          ))}
        </ul>

        {noCacheCount > 0 && (
          <NoCacheToggle showAll={showAll} count={noCacheCount} label="系列" onToggle={onToggleShowAll} />
        )}

        {standalone.length > 0 && (
          <p className="px-3 pb-1 pt-3 text-xs font-medium uppercase tracking-wide text-neutral-400">
            未归组单集（{standalone.length}）
          </p>
        )}
        <ul className="space-y-1 px-2">
          {standalone.map((st) => (
            <StandaloneRow
              key={st.videoId}
              item={st}
              active={selection?.kind === 'standalone' && selection.id === st.videoId}
              onSelect={() => onSelect({ kind: 'standalone', id: st.videoId })}
            />
          ))}
        </ul>

        {standaloneNoCacheCount > 0 && (
          <NoCacheToggle
            showAll={standaloneShowAll}
            count={standaloneNoCacheCount}
            label="单集"
            onToggle={onToggleStandaloneShowAll}
          />
        )}

        {empty && (
          <div className="flex flex-col items-center justify-center gap-1 px-6 py-10 text-center">
            <p className="text-sm text-neutral-500">
              {query ? '没有匹配的缓存' : noCacheCount > 0 ? '还没有系列产生缓存' : '暂无缓存'}
            </p>
            <p className="text-xs text-neutral-400">
              {query ? '换个关键词试试' : '播放过含内封字幕的视频后，这里会显示其字幕缓存与播放记忆。'}
            </p>
          </div>
        )}
      </div>

      {confirmOrphans && (
        <ConfirmDialog
          title="清理孤儿缓存"
          description="将删除以下不在库中的残留缓存文件："
          items={orphans ? orphanItems(orphans) : []}
          onClose={() => setConfirmOrphans(false)}
          onConfirm={() =>
            (async () => {
              try {
                const { cleared } = await clearOrphanCache()
                await onChanged()
                notify(`已清理 ${cleared} 个孤儿缓存文件`)
              } catch (err) {
                notify(err instanceof Error ? err.message : '清理失败', 'error')
              }
            })().then(() => setConfirmOrphans(false))
          }
        />
      )}
    </div>
  )
}

function orphanItems(o: CacheOrphans): { label: string; detail: string }[] {
  const out: { label: string; detail: string }[] = []
  const push = (label: string, c: CacheOrphans['cover']) => {
    if (c.orphans > 0) out.push({ label, detail: `${c.orphans} 个 · ${formatBytes(c.orphan_bytes)}` })
  }
  push('封面', o.cover)
  push('缩略图', o.thumb)
  push('字幕', o.subtitle)
  push('Remux', o.remux)
  return out
}

// NoCacheToggle is the one-line「显示无缓存的…」switch shared by the series and
// standalone sections: it reveals (or hides) the grayed no-cache rows.
function NoCacheToggle({
  showAll,
  count,
  label,
  onToggle,
}: {
  showAll: boolean
  count: number
  label: string
  onToggle: () => void
}) {
  return (
    <button
      onClick={onToggle}
      className="mx-3 my-2 flex w-[calc(100%-1.5rem)] items-center justify-center gap-1 rounded-md border border-dashed border-neutral-300 px-2 py-1.5 text-xs text-neutral-500 hover:border-blue-400 hover:text-blue-600"
    >
      {showAll ? `收起无缓存的${label}` : `显示无缓存的${label}（${count} 个）`}
    </button>
  )
}

// GlobalRow is a pinned top-level cache item (orphan / remux): one row that
// opens its confirm dialog, styled like the series rows for a uniform list.
function GlobalRow({
  icon,
  title,
  sub,
  onAction,
}: {
  icon: ReactNode
  title: string
  sub: string
  onAction: () => void
}) {
  return (
    <button
      onClick={onAction}
      className="flex w-full items-center gap-2.5 rounded-md border border-neutral-200 bg-white px-2.5 py-2 text-left hover:bg-neutral-50"
    >
      <div className="flex h-10 w-16 shrink-0 items-center justify-center rounded border border-neutral-200 bg-neutral-100 text-neutral-400">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-neutral-700">{title}</p>
        <p className="mt-0.5 truncate text-xs text-neutral-400">{sub}</p>
      </div>
      <span className="flex shrink-0 items-center gap-1 rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-700">
        <Eraser className="size-3" /> 清理
      </span>
    </button>
  )
}

function SeriesRow({
  info,
  active,
  onSelect,
}: {
  info: SeriesCacheInfo
  active: boolean
  onSelect: () => void
}) {
  const hasAny = hasCache(info)
  return (
    <li>
      <button
        onClick={onSelect}
        className={`flex w-full items-center gap-2.5 rounded-md border px-2.5 py-2 text-left transition-colors ${
          active ? 'border-blue-500 bg-blue-50' : 'border-neutral-200 bg-white hover:bg-neutral-50'
        } ${hasAny ? '' : 'opacity-50'}`}
      >
        <div className="h-10 w-16 shrink-0 overflow-hidden rounded border border-neutral-200 bg-neutral-100">
          {info.series.member_count > 0 ? (
            <img
              src={seriesPosterUrl(info.series.id)}
              alt={info.series.name}
              loading="lazy"
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-neutral-300">
              <Layers className="size-4" />
            </div>
          )}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-neutral-800">{info.series.name}</p>
          <div className="mt-1 flex flex-wrap gap-1">
            {info.subtitleFiles > 0 && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">
                字幕 {info.subtitleFiles} 轨 · {formatBytes(info.subtitleBytes)}
              </span>
            )}
            {info.remuxFiles > 0 && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">
                Remux {info.remuxFiles} 个 · {formatBytes(info.remuxBytes)}
              </span>
            )}
            {(info.memberPrefs > 0 || info.hasSeriesPrefs) && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">记忆</span>
            )}
            {!hasAny && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-400">无缓存</span>
            )}
          </div>
        </div>
      </button>
    </li>
  )
}

function StandaloneRow({
  item,
  active,
  onSelect,
}: {
  item: StandaloneCacheInfo
  active: boolean
  onSelect: () => void
}) {
  const hasAny = hasStandaloneCache(item)
  return (
    <li>
      <button
        onClick={onSelect}
        className={`flex w-full items-center gap-2.5 rounded-md border px-2.5 py-2 text-left transition-colors ${
          active ? 'border-blue-500 bg-blue-50' : 'border-neutral-200 bg-white hover:bg-neutral-50'
        } ${hasAny ? '' : 'opacity-50'}`}
      >
        <div className="h-10 w-16 shrink-0 overflow-hidden rounded border border-neutral-200 bg-neutral-100">
          {item.hasThumb ? (
            <img
              src={coverUrl(item.videoId, true)}
              alt={item.title}
              loading="lazy"
              className="h-full w-full object-cover"
              onError={(e) => {
                e.currentTarget.style.display = 'none'
              }}
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-neutral-300">
              <Layers className="size-4" />
            </div>
          )}
        </div>
        <div className="min-w-0 flex-1">
          <p className={`truncate text-sm font-medium ${hasAny ? 'text-neutral-800' : 'text-neutral-500'}`}>
            {item.title}
          </p>
          <div className="mt-1 flex flex-wrap gap-1">
            {item.group && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">
                字幕 {item.group.files.length} 轨 · {formatBytes(item.group.bytes)}
              </span>
            )}
            {item.remuxFiles > 0 && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">
                Remux {item.remuxFiles} 个 · {formatBytes(item.remuxBytes)}
              </span>
            )}
            {item.prefs && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-500">记忆</span>
            )}
            {!hasAny && (
              <span className="rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] text-neutral-400">无缓存</span>
            )}
          </div>
        </div>
      </button>
    </li>
  )
}