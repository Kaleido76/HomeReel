export type GridState = {
  view: 'all' | 'standalone' | 'series'
  q: string
  page: number
  tags: string[]
}

export const emptyFilters = { tags: [] }

export const viewTabs: { value: GridState['view']; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'standalone', label: '单集' },
  { value: 'series', label: '系列' },
]

export type ListSelection = { type: 'video' | 'series'; id: string } | null
