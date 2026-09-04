import type {
  CacheOverview,
  CacheRemuxGroup,
  PlaybackPrefsCacheEntry,
  SeriesPrefsCacheEntry,
  SubtitleCacheGroup,
} from '../../../api/cache'
import type { Series } from '../../../api/series'
import type { Video } from '../../../api/videos'

// CacheSelection identifies the currently inspected management unit: a series
// (the dominant unit) or a lone standalone video that owns its caches.
export type CacheSelection = { kind: 'series'; id: string } | { kind: 'standalone'; id: string }

// SeriesCacheInfo aggregates one series' cache state from the flat overview.
export interface SeriesCacheInfo {
  series: Series
  subtitleFiles: number
  subtitleBytes: number
  hasSeriesPrefs: boolean
  memberPrefs: number
  remuxFiles: number
  remuxBytes: number
}

// StandaloneCacheInfo is a video outside any series. Every standalone video of
// the library appears (matching the library's 单集 view); group/prefs/remux are
// set only when it owns that cache.
export interface StandaloneCacheInfo {
  videoId: string
  title: string
  hasThumb: boolean
  group?: SubtitleCacheGroup
  prefs?: PlaybackPrefsCacheEntry
  remuxFiles: number
  remuxBytes: number
}

// CacheModel is the whole cache page's data: every series (with or without
// cache) plus every standalone video, plus the per-video lookup maps the detail
// panes merge against series members.
export interface CacheModel {
  series: SeriesCacheInfo[]
  standalone: StandaloneCacheInfo[]
  subtitlesByVideo: Map<string, SubtitleCacheGroup>
  remuxByVideo: Map<string, CacheRemuxGroup>
  prefsByVideo: Map<string, PlaybackPrefsCacheEntry>
  seriesPrefs: Map<string, SeriesPrefsCacheEntry>
}

// hasCache decides whether a series appears in the default (cached-only) list.
export function hasCache(s: SeriesCacheInfo): boolean {
  return s.subtitleFiles > 0 || s.memberPrefs > 0 || s.hasSeriesPrefs || s.remuxFiles > 0
}

export function hasStandaloneCache(s: StandaloneCacheInfo): boolean {
  return !!s.group || !!s.prefs || s.remuxFiles > 0
}

// buildCacheModel turns the flat cache overview plus the series index and the
// full standalone-video list into the hierarchical model the page renders. A
// video's show_id is 1:1 with its series (grouping.go binds every video under a
// series root and assigns standalone otherwise), so each show_id maps to exactly
// one series.
export function buildCacheModel(
  overview: CacheOverview | undefined,
  allSeries: Series[],
  standaloneVideos: Video[],
): CacheModel {
  const showToSeries = new Map<string, string>()
  for (const s of allSeries) {
    if (s.show_id) showToSeries.set(s.show_id, s.id)
  }

  const series = new Map<string, SeriesCacheInfo>()
  for (const s of allSeries) {
    series.set(s.id, {
      series: s,
      subtitleFiles: 0,
      subtitleBytes: 0,
      hasSeriesPrefs: false,
      memberPrefs: 0,
      remuxFiles: 0,
      remuxBytes: 0,
    })
  }

  const subtitlesByVideo = new Map<string, SubtitleCacheGroup>()
  const remuxByVideo = new Map<string, CacheRemuxGroup>()
  const prefsByVideo = new Map<string, PlaybackPrefsCacheEntry>()
  const seriesPrefs = new Map<string, SeriesPrefsCacheEntry>()
  const standalone = new Map<string, StandaloneCacheInfo>()
  for (const v of standaloneVideos) {
    standalone.set(v.id, { videoId: v.id, title: v.title, hasThumb: !!v.thumb_path, remuxFiles: 0, remuxBytes: 0 })
  }
  // A cached video the standalone list missed (e.g. still loading) still shows.
  const ensureStandalone = (videoId: string, title: string): StandaloneCacheInfo => {
    let st = standalone.get(videoId)
    if (!st) {
      st = { videoId, title, hasThumb: false, remuxFiles: 0, remuxBytes: 0 }
      standalone.set(videoId, st)
    }
    return st
  }

  for (const g of overview?.subtitles ?? []) {
    subtitlesByVideo.set(g.video_id, g)
    const sid = g.show_id ? showToSeries.get(g.show_id) : undefined
    const si = sid ? series.get(sid) : undefined
    if (si) {
      si.subtitleFiles += g.files.length
      si.subtitleBytes += g.bytes
    } else {
      ensureStandalone(g.video_id, g.title).group = g
    }
  }

  for (const r of overview?.remuxes ?? []) {
    remuxByVideo.set(r.video_id, r)
    const sid = r.show_id ? showToSeries.get(r.show_id) : undefined
    const si = sid ? series.get(sid) : undefined
    if (si) {
      si.remuxFiles += r.files
      si.remuxBytes += r.bytes
    } else {
      const st = ensureStandalone(r.video_id, r.title)
      st.remuxFiles += r.files
      st.remuxBytes += r.bytes
    }
  }

  for (const p of overview?.prefs ?? []) {
    prefsByVideo.set(p.video_id, p)
    const sid = p.show_id ? showToSeries.get(p.show_id) : undefined
    const si = sid ? series.get(sid) : undefined
    if (si) {
      si.memberPrefs++
    } else {
      ensureStandalone(p.video_id, p.title).prefs = p
    }
  }

  for (const sp of overview?.series_prefs ?? []) {
    seriesPrefs.set(sp.series_id, sp)
    const si = series.get(sp.series_id)
    if (si) si.hasSeriesPrefs = true
  }

  const standaloneList = [...standalone.values()]
  standaloneList.sort((a, b) => {
    const ac = hasStandaloneCache(a) ? 0 : 1
    const bc = hasStandaloneCache(b) ? 0 : 1
    if (ac !== bc) return ac - bc
    return a.title.localeCompare(b.title, 'zh-Hans-CN')
  })

  return {
    series: [...series.values()],
    standalone: standaloneList,
    subtitlesByVideo,
    remuxByVideo,
    prefsByVideo,
    seriesPrefs,
  }
}

export function orphanTotal(o: CacheOverview['orphans']): number {
  return o.cover.orphans + o.thumb.orphans + o.subtitle.orphans + o.remux.orphans
}

export function orphanBytesTotal(o: CacheOverview['orphans']): number {
  return o.cover.orphan_bytes + o.thumb.orphan_bytes + o.subtitle.orphan_bytes + o.remux.orphan_bytes
}

// prefsSummary turns a video's playback selection cache into a compact
// description of what would be re-applied on the next play.
export function prefsSummary(p: PlaybackPrefsCacheEntry): string {
  const parts: string[] = []
  if (typeof p.audio_track === 'number') parts.push(`音轨 ${p.audio_track + 1}`)
  if (typeof p.subtitle_id === 'string') {
    if (p.subtitle_id === '') parts.push('字幕 关闭')
    else parts.push(`字幕 ${p.subtitle_id === 'sidecar' ? '侧边文件' : `内封轨 ${p.subtitle_id.replace(/^e/, '')}`}`)
  }
  if (typeof p.volume === 'number') parts.push(`音量 ${Math.round(p.volume * 100)}%${p.muted ? '（静音）' : ''}`)
  return parts.length > 0 ? parts.join(' · ') : '（仅记录了部分偏好）'
}

// seriesPrefsSummary turns a series' shared playback selection cache into a
// compact description of what every episode would auto-apply (tracks by name).
export function seriesPrefsSummary(p: SeriesPrefsCacheEntry): string {
  const parts: string[] = []
  if (typeof p.audio_track_name === 'string') parts.push(`音轨 ${p.audio_track_name}`)
  if (typeof p.subtitle_name === 'string') parts.push(`字幕 ${p.subtitle_name === '' ? '关闭' : p.subtitle_name}`)
  if (typeof p.volume === 'number') parts.push(`音量 ${Math.round(p.volume * 100)}%${p.muted ? '（静音）' : ''}`)
  return parts.length > 0 ? parts.join(' · ') : '（仅记录了部分偏好）'
}