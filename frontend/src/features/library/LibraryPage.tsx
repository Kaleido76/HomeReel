import { useLocation, useNavigate } from '@tanstack/react-router'
import { StandaloneGrid } from './VideoGrid'
import { SeriesPage } from '../series/SeriesPage'

// VideoLibraryPage is the unified library organised into two views: standalone
// videos (单集) and series (系列). The active view lives in the URL (?view=) so
// it survives refresh and in-tab back/forward.
export function LibraryPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const search = (location.search ?? {}) as { view?: string }
  const view = search.view === 'series' ? 'series' : 'standalone'

  const tabs: { value: typeof view; label: string }[] = [
    { value: 'standalone', label: '单集' },
    { value: 'series', label: '系列' },
  ]

  function setView(v: typeof view) {
    navigate({ to: '/library', search: { ...search, view: v } })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900">视频库</h1>
      </div>
      <div className="flex w-fit gap-1 rounded-lg border border-neutral-200 bg-white p-1">
        {tabs.map((t) => (
          <button
            key={t.value}
            onClick={() => setView(t.value)}
            className={
              view === t.value
                ? 'rounded-md bg-neutral-900 px-4 py-1.5 text-sm font-medium text-white'
                : 'rounded-md px-4 py-1.5 text-sm text-neutral-600 hover:bg-neutral-100'
            }
          >
            {t.label}
          </button>
        ))}
      </div>
      {view === 'standalone' ? (
        <StandaloneGrid />
      ) : (
        <SeriesPage embedded />
      )}
    </div>
  )
}
