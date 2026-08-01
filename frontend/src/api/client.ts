export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  const resp = await fetch(path, { ...init, headers, credentials: 'include' })
  if (!resp.ok) {
    let code = 'error'
    let message = resp.statusText
    try {
      const body = (await resp.json()) as { error?: { code?: string; message?: string } }
      code = body.error?.code ?? code
      message = body.error?.message ?? message
    } catch {
      // non-JSON error body
    }
    throw new ApiError(resp.status, code, message)
  }
  return resp.json() as Promise<T>
}
