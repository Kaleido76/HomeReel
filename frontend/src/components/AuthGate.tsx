import { useAuth } from '../features/auth/auth'
import { LoginPage } from '../features/auth/LoginPage'
import { AppShell } from './AppShell'
import { FullScreenLoader } from './FullScreenLoader'

// AuthGate 把「登录态」与「页签宿主」解耦：未认证直接渲染登录页，
// 认证后挂载 AppShell（内含每个页签各自的 Router）。登录/登出不再依赖
// 路由导航，避免时序问题；登出会卸载整个页签宿主，重置所有页签状态。
export function AuthGate() {
  const { authenticated, isLoading } = useAuth()

  if (isLoading) {
    return <FullScreenLoader />
  }

  if (!authenticated) {
    return <LoginPage />
  }

  return <AppShell />
}
