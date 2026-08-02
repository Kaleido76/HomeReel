import { useState, type FormEvent } from 'react'
import { Loader2 } from 'lucide-react'
import { useAuth } from './auth'
import { ApiError } from '../../api/client'

export function LoginPage() {
  const { login } = useAuth()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      await login(password)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '登录失败，请稍后再试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950 px-4">
      <div className="w-full max-w-sm rounded-xl border border-neutral-800 bg-neutral-900 p-6 shadow-xl">
        <div className="mb-6 text-center">
          <h1 className="text-xl font-semibold text-neutral-50">HomeReel</h1>
          <p className="mt-1 text-sm text-neutral-400">请输入访问口令</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="访问口令"
            autoFocus
            required
            className="w-full rounded-lg border border-neutral-700 bg-neutral-800 px-3 py-2 text-neutral-100 placeholder-neutral-500 outline-none focus:border-neutral-500"
          />
          {error && <p className="text-sm text-red-400">{error}</p>}
          <button
            type="submit"
            disabled={submitting}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-indigo-600 px-3 py-2 font-medium text-white transition-colors hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting && <Loader2 className="size-4 animate-spin" />}
            登录
          </button>
        </form>
      </div>
    </div>
  )
}
