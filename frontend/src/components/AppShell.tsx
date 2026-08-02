import { LogOut, Video } from 'lucide-react'
import { useAuth } from '../features/auth/auth'
import { TabBar } from '../tabs/TabBar'
import { TabHost } from '../tabs/TabHost'
import { activate } from '../tabs/manager'

export function AppShell() {
  const { logout } = useAuth()

  return (
    <div className="flex h-dvh flex-col bg-neutral-50">
      <header className="shrink-0 border-b border-neutral-200 bg-white">
        <div className="mx-auto flex h-14 w-full max-w-[1920px] items-center justify-between gap-4 px-4 sm:px-6 xl:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <button
              onClick={() => activate('home')}
              className="flex shrink-0 items-center gap-2 font-semibold text-neutral-900"
            >
              <Video className="size-5 text-blue-600" />
              <span className="hidden sm:inline">HomeReel</span>
            </button>
            <TabBar />
          </div>
          <button
            onClick={logout}
            className="flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
          >
            <LogOut className="size-4" />
            <span className="hidden md:inline">退出登录</span>
          </button>
        </div>
      </header>
      <main className="min-h-0 flex-1">
        <TabHost />
      </main>
    </div>
  )
}
