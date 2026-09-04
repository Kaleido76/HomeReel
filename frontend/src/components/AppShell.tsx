import { LogOut } from 'lucide-react'
import { useAuth } from '../features/auth/auth'
import { JobsIndicator } from '../features/jobs/JobsIndicator'
import { TabBar } from '../tabs/TabBar'
import { TabHost } from '../tabs/TabHost'
import { activate } from '../tabs/manager'
import { NotificationProvider } from './NotificationProvider'

export function AppShell() {
  const { logout } = useAuth()

  return (
    <NotificationProvider>
      <div className="flex h-dvh flex-col bg-neutral-50">
        <header className="relative z-50 shrink-0 border-b border-neutral-200 bg-white">
          <div className="flex h-14 w-full items-center justify-between gap-4 px-4 sm:px-6">
            <div className="flex min-w-0 items-center gap-3">
              <button
                onClick={() => activate('home')}
                className="hidden shrink-0 items-center font-bold text-neutral-900 sm:flex"
              >
                HomeReel
              </button>
              <TabBar />
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <JobsIndicator />
              <button
                onClick={logout}
                className="flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
              >
                <LogOut className="size-4" />
                <span className="hidden md:inline">退出登录</span>
              </button>
            </div>
          </div>
        </header>
        <main className="min-h-0 flex-1">
          <TabHost />
        </main>
      </div>
    </NotificationProvider>
  )
}
