import { api } from './client'

export interface AuthStatus {
  authenticated: boolean
}

export function fetchAuthStatus(): Promise<AuthStatus> {
  return api<AuthStatus>('/api/auth/status')
}

export function login(password: string): Promise<AuthStatus> {
  return api<AuthStatus>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function logout(): Promise<AuthStatus> {
  return api<AuthStatus>('/api/auth/logout', { method: 'POST' })
}

export function fetchMe(): Promise<{ user: string }> {
  return api<{ user: string }>('/api/me')
}
