import type { ComponentType } from 'react'
import { DatabaseBackup, Factory, type LucideIcon } from 'lucide-react'
import { FormatFactoryPage } from './format/FormatFactoryPage'
import { CacheManagerPage } from './cache/CacheManagerPage'

// ToolDef is one tool in the 工具 tab's left rail. New tools register here and
// appear in the rail automatically; the ToolsPage keeps every visited tool
// mounted so switching preserves its state (same keep-alive idea as the tabs).
export interface ToolDef {
  id: string
  label: string
  icon: LucideIcon
  component: ComponentType
}

export const TOOL_DEFS: ToolDef[] = [
  { id: 'format', label: '格式工厂', icon: Factory, component: FormatFactoryPage },
  { id: 'cache', label: '缓存管理', icon: DatabaseBackup, component: CacheManagerPage },
]
