import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { fetchSeries } from '../../api/series'
import { SeriesCard } from './SeriesCard'

export function SeriesPage({ embedded = false }: { embedded?: boolean }) {
  const series = useQuery({ queryKey: ['series'], queryFn: fetchSeries })

  if (!embedded && series.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center text-neutral-400">
        <Loader2 className="size-6 animate-spin" />
      </div>
    )
  }

  if (series.isError) {
    return (
      <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
        {series.error.message}
      </div>
    )
  }

  if (!series.data) return null

  return (
    <div>
      {series.data.series.length === 0 ? (
        <div className="rounded-xl border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          暂无系列。同一目录下多个相近命名的视频（如 S01E01、S01E02 或「第1部」「第2部」）会被自动归为一季/一部的系列。
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
          {series.data.series.map((s) => (
            <SeriesCard key={s.id} series={s} />
          ))}
        </div>
      )}
    </div>
  )
}
