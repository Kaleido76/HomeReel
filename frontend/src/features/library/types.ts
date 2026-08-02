export const sortOptions = [
  { value: 'date', label: '最近添加' },
  { value: 'title', label: '标题' },
  { value: 'duration', label: '时长' },
  { value: 'name', label: '文件名' },
  { value: 'rating', label: '评分' },
] as const

export type GridState = {
  view: 'all' | 'standalone' | 'series'
  q: string
  sort: (typeof sortOptions)[number]['value']
  page: number
}

export const viewTabs: { value: GridState['view']; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'standalone', label: '单集' },
  { value: 'series', label: '系列' },
]

export type ListSelection = { type: 'video' | 'series'; id: string } | null
