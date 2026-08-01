import { RouterProvider } from '@tanstack/react-router'
import { useAuth } from '../features/auth/auth'
import { LoginPage } from '../features/auth/LoginPage'
import { router } from '../router'
import { FullScreenLoader } from './FullScreenLoader'

// AuthGate 把「登录态」与「路由器」解耦：未认证直接渲染登录页，
// 认证后挂载 RouterProvider。登录/登出不再依赖路由导航，避免时序问题。
export function AuthGate() {
  const { authenticated, isLoading } = useAuth()

  if (isLoading) {
    return <FullScreenLoader />
  }

  if (!authenticated) {
    return <LoginPage />
  }

  return <RouterProvider router={router} />
}
