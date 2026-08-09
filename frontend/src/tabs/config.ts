import { FolderTree, Home, MonitorPlay, Search, Wrench, type LucideIcon } from 'lucide-react'

// TabId is one of the top-level "browser tabs". Each tab owns an independent
// router instance whose component tree stays mounted while hidden, so switching
// tabs never loses view state (scroll, filters, the player, uploads…).
export type TabId = 'home' | 'library' | 'search' | 'tools' | 'files'

export interface TabDef {
  id: TabId
  label: string
  root: string
  icon: LucideIcon
}

export const TAB_DEFS: TabDef[] = [
  { id: 'home', label: '首页', root: '/', icon: Home },
  { id: 'library', label: '视频库', root: '/library', icon: MonitorPlay },
  { id: 'search', label: '搜索', root: '/search', icon: Search },
  { id: 'tools', label: '工具', root: '/tools', icon: Wrench },
  { id: 'files', label: '文件', root: '/files', icon: FolderTree },
]

export const TAB_ROOTS: Record<TabId, string> = {
  home: '/',
  library: '/library',
  search: '/search',
  tools: '/tools',
  files: '/files',
}

// tabFromPath maps a URL pathname to the tab that owns it. Deep routes such as
// the player (/library/video/:id) and series details (/series/:id) belong to the
// library tab, so clicking a video anywhere jumps to that tab's history.
export function tabFromPath(pathname: string): TabId {
  if (pathname === '/' || pathname === '') return 'home'
  if (pathname.startsWith('/library') || pathname.startsWith('/series')) return 'library'
  if (pathname.startsWith('/search')) return 'search'
  if (pathname.startsWith('/tools')) return 'tools'
  if (pathname.startsWith('/files')) return 'files'
  return 'home'
}
