import type { ReactNode } from 'react'

// MediaGrid is the shared responsive wall used by library / home / search.
// Cards declare how many grid rows they span (VideoCard: 2, SeriesCard: 3) so
// landscape 16:9 items and portrait 2:3 posters pack densely together
// (grid-flow-dense back-fills the holes a portrait leaves next to it).
// Column count scales with the viewport: 2 on phones up to 8+ on 2K/4K walls.
export function MediaGrid({ children }: { children: ReactNode }) {
  return (
    <div className="grid auto-rows-[5rem] grid-flow-dense grid-cols-2 gap-3 sm:auto-rows-[5.5rem] sm:grid-cols-3 md:auto-rows-[6rem] md:grid-cols-4 lg:auto-rows-[6.25rem] lg:grid-cols-5 xl:auto-rows-[6.5rem] xl:grid-cols-6 2xl:auto-rows-[6.75rem] 2xl:grid-cols-7 3xl:auto-rows-[7rem] 3xl:grid-cols-8">
      {children}
    </div>
  )
}
