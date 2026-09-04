import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

type NotifyType = 'success' | 'warning' | 'error'

interface Notification {
  id: number
  message: string
  type: NotifyType
  holdMs: number
}

interface NotifyContextValue {
  notify: (message: string, type?: NotifyType, duration?: number) => void
  dismiss: (id: number) => void
}

const NotifyContext = createContext<NotifyContextValue | null>(null)

let nextId = 0

// Default display durations by type (ms). Error stays longer for readability.
const DEFAULT_DURATION: Record<NotifyType, number> = {
  success: 3500,
  warning: 4000,
  error: 5000,
}

// Animation durations (ms) — must match the CSS @keyframes below.
const SLIDE_IN_MS = 200
const SLIDE_OUT_MS = 200

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Notification[]>([])
  const timeouts = useRef(new Map<number, ReturnType<typeof setTimeout>>())

  const dismiss = useCallback((id: number) => {
    const t = timeouts.current.get(id)
    if (t) {
      clearTimeout(t)
      timeouts.current.delete(id)
    }
    setItems((prev) => prev.filter((n) => n.id !== id))
  }, [])

  const notify = useCallback(
    (message: string, type: NotifyType = 'success', duration?: number) => {
      const id = nextId++
      const dur = duration ?? DEFAULT_DURATION[type]

      // Clear any existing timeout for rapid re-fire.
      const existing = timeouts.current.get(id)
      if (existing) {
        clearTimeout(existing)
        timeouts.current.delete(id)
      }

      setItems((prev) => [...prev, { id, message, type, holdMs: dur }])

      // Slide-in + hold + slide-out = SLIDE_IN_MS + dur + SLIDE_OUT_MS.
      const total = SLIDE_IN_MS + dur + SLIDE_OUT_MS
      timeouts.current.set(
        id,
        setTimeout(() => {
          timeouts.current.delete(id)
          setItems((prev) => prev.filter((n) => n.id !== id))
        }, total),
      )
    },
    [],
  )

  return (
    <NotifyContext.Provider value={{ notify, dismiss }}>
      {children}
      {items.length > 0 && (
        <div className="pointer-events-none fixed inset-x-0 top-[60px] z-[99999] flex flex-col items-center gap-2 px-4 sm:px-6">
          {items.map((n) => (
            <NotificationBanner key={n.id} notification={n} />
          ))}
        </div>
      )}
    </NotifyContext.Provider>
  )
}

export function useNotify(): NotifyContextValue {
  const ctx = useContext(NotifyContext)
  if (!ctx) throw new Error('useNotify must be used within NotificationProvider')
  return ctx
}

function NotificationBanner({
  notification,
}: {
  notification: Notification
}) {
  const { holdMs, message, type } = notification

  // Determine animation class based on type.
  const animClass =
    type === 'error' ? 'notify-slide-error' : type === 'warning' ? 'notify-slide-warning' : 'notify-slide-success'

  return (
    <div
      className={`${animClass} pointer-events-auto flex w-full max-w-[28rem] items-center rounded-[6px] px-4 py-2 text-xs ring-1 ring-black/[0.06] shadow sm:text-sm ${BG_CLASS[type]}`}
      style={{ '--notify-hold': `${holdMs}ms` } as React.CSSProperties}
      role="alert"
    >
      <span className="min-w-0 flex-1 text-neutral-700">{message}</span>
    </div>
  )
}

const BG_CLASS: Record<NotifyType, string> = {
  success: 'bg-green-50',
  warning: 'bg-orange-50',
  error: 'bg-red-50',
}
