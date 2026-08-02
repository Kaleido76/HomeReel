import { Link, Outlet } from '@tanstack/react-router'
import {
  FolderOpen,
  Home,
  LogOut,
  MonitorPlay,
  Search,
  Video,
  type LucideIcon,
} from 'lucide-react'
import { useAuth } from '../features/auth/auth'

const linkBase = 'rounded-lg px-3 py-1.5 text-sm transition-colors'
const linkActive = 'bg-neutral-100 font-medium text-neutral-900'
const linkInactive = 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900'

const navItems: { to: string; label: string; icon: LucideIcon; end?: boolean }[] = [
  { to: '/', label: '首页', icon: Home, end: true },
  { to: '/library', label: '视频库', icon: MonitorPlay },
  { to: '/search', label: '搜索', icon: Search },
  { to: '/explorer', label: '文件', icon: FolderOpen },
]

export function AppShell() {
  const { logout } = useAuth()

  return (
    <div className="flex min-h-screen flex-col bg-neutral-50">
      <header className="border-b border-neutral-200 bg-white">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between px-4">
          <div className="flex min-w-0 items-center gap-4">
            <Link to="/" className="flex shrink-0 items-center gap-2 font-semibold text-neutral-900">
              <Video className="size-5 text-indigo-600" />
              VideoMesh
            </Link>
            <nav className="flex min-w-0 items-center gap-1 overflow-x-auto">
              {navItems.map((item) => (
                <Link
                  key={item.to}
                  to={item.to}
                  className={`${linkBase} shrink-0`}
                  activeOptions={item.end ? { exact: true } : undefined}
                  activeProps={{ className: linkActive }}
                  inactiveProps={{ className: linkInactive }}
                >
                  <span className="flex items-center gap-1.5">
                    <item.icon className="size-4" /> {item.label}
                  </span>
                </Link>
              ))}
            </nav>
          </div>
          <button
            onClick={logout}
            className="flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
          >
            <LogOut className="size-4" />
            退出登录
          </button>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
