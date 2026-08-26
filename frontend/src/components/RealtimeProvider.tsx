import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { realtime, type RealtimeClient, type RealtimeStatus } from '../api/realtime'
import { useAuth } from '../features/auth/auth'

// RealtimeProvider owns the WebSocket lifecycle for the whole app: it connects
// while a session exists and disconnects on logout. Everything below it can
// subscribe to pushes and fire RPC requests through the shared client.

interface RealtimeContextValue {
  client: RealtimeClient
  status: RealtimeStatus
}

const RealtimeContext = createContext<RealtimeContextValue | null>(null)

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const { authenticated } = useAuth()

  useEffect(() => {
    if (authenticated) {
      realtime.connect()
    } else {
      realtime.disconnect()
    }
  }, [authenticated])

  const [status, setStatus] = useState<RealtimeStatus>(realtime.getStatus())
  useEffect(() => realtime.onStatus(setStatus), [])

  return <RealtimeContext.Provider value={{ client: realtime, status }}>{children}</RealtimeContext.Provider>
}

export function useRealtime(): RealtimeContextValue {
  const ctx = useContext(RealtimeContext)
  if (!ctx) {
    throw new Error('useRealtime 必须在 RealtimeProvider 内使用')
  }
  return ctx
}

/**
 * Subscribes a handler to a server push message type. The handler runs on
 * every matching frame while the component is mounted. The latest handler is
 * kept in a ref, so inline closures never cause resubscribes on re-render.
 */
export function useRealtimeMessage(type: string, handler: ((data: unknown) => void) | null | undefined): void {
  const { client } = useRealtime()
  const handlerRef = useRef(handler)
  handlerRef.current = handler
  useEffect(() => client.on(type, (data) => handlerRef.current?.(data)), [client, type])
}