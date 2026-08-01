import { createContext, useContext, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchAuthStatus, login as loginApi, logout as logoutApi } from '../../api/auth'

interface AuthState {
  authenticated: boolean
  isLoading: boolean
  login: (password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

const statusKey = ['auth', 'status'] as const

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: statusKey,
    queryFn: fetchAuthStatus,
    retry: false,
    staleTime: 60_000,
  })

  // 登录/登出的结果由服务端响应确定性地写入缓存，而不是异步 refetch 回读，
  // 避免连续操作时多次状态请求乱序造成界面与真实登录态不一致。
  const login = async (password: string) => {
    const status = await loginApi(password)
    queryClient.setQueryData(statusKey, status)
  }

  const logout = async () => {
    const status = await logoutApi()
    queryClient.setQueryData(statusKey, status)
  }

  const value: AuthState = {
    authenticated: query.data?.authenticated ?? false,
    isLoading: query.isPending,
    login,
    logout,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth 必须在 AuthProvider 内使用')
  }
  return ctx
}