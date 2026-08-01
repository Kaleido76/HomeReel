import { Link, Outlet } from '@tanstack/react-router'
import { FolderOpen, Home, LogOut, Video } from 'lucide-react'
import { useAuth } from '../features/auth/auth'

const linkBase = 'rounded-lg px-3 py-1.5 text-sm transition-colors'
const linkActive = 'bg-neutral-100 font-medium text-neutral-900'
const linkInactive = 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900'

export function AppShell() {
  const { logout } = useAuth()

  return (
    <div className="flex min-h-screen flex-col bg-neutral-50">
      <header className="border-b border-neutral-200 bg-white">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between px-4">
          <div className="flex items-center gap-4">
            <Link to="/" className="flex items-center gap-2 font-semibold text-neutral-900">
              <Video className="size-5 text-indigo-600" />
              VideoMesh
            </Link>
            <nav className="flex items-center gap-1">
              <Link
                to="/"
                className={linkBase}
                activeProps={{ className: linkActive }}
                inactiveProps={{ className: linkInactive }}
              >
                <span className="flex items-center gap-1.5">
                  <Home className="size-4" /> 首页
                </span>
              </Link>
              <Link
                to="/explorer"
                search={{ storageId: '', path: '' }}
                className={linkBase}
                activeProps={{ className: linkActive }}
                inactiveProps={{ className: linkInactive }}
              >
                <span className="flex items-center gap-1.5">
                  <FolderOpen className="size-4" /> 文件管理
                </span>
              </Link>
            </nav>
          </div>
          <button
            onClick={logout}
            className="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
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
