import { Clapperboard } from 'lucide-react'
import { useParams, useSearch } from '@tanstack/react-router'
import { openLibrary } from '../../tabs/manager'
import { PlayerPane } from './PlayerPane'

// PlayerPage is the player tab's only page. The route carries the playing video
// (/player/:videoId) and optionally the series context (?series=) that drives
// the episode list; the root route (/player, no videoId) is the empty state
// shown before anything has been played.
//
// Navigation (prev/next, episode clicks) is handled entirely inside PlayerPane
// via the manager — this page is a thin route ↔ component bridge.
export function PlayerPage() {
  const { videoId } = useParams({ strict: false })
  const search = useSearch({ strict: false }) as { series?: string }
  const seriesId = search.series || undefined

  if (!videoId) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-neutral-50 p-6 text-center">
        <Clapperboard className="size-12 text-neutral-300" />
        <p className="text-sm text-neutral-600">还没有播放中的视频。请在视频库中选择一个开始播放。</p>
        <button
          onClick={openLibrary}
          className="flex items-center gap-1.5 rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700"
        >
          前往视频库
        </button>
      </div>
    )
  }

  return <PlayerPane videoId={videoId} seriesId={seriesId} />
}
