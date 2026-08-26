// RealtimeClient is the frontend half of the WebSocket channel (ADR-021).
//
// One singleton serves the whole app. It multiplexes two kinds of traffic on a
// single connection:
//   - server→client pushes: fire-and-forget notifications routed to handlers
//     registered by message type (see on());
//   - client→server RPC: request(type, payload) resolves with the matching
//     "<type>.result" frame (correlated by id) or rejects with "<type>.error".
//
// The connection follows the session: the app calls connect() once logged in
// and disconnect() on logout. It auto-reconnects with exponential backoff and
// resumes on tab focus.

export interface RealtimeEnvelope<T = unknown> {
  id?: string
  type: string
  data?: T
}

export type RealtimeStatus = 'disconnected' | 'connecting' | 'connected'

export class RealtimeError extends Error {
  code: string

  constructor(code: string, message: string) {
    super(message)
    this.name = 'RealtimeError'
    this.code = code
  }
}

type MessageHandler = (data: unknown) => void
type StatusListener = (status: RealtimeStatus) => void
type PendingRequest = {
  resolve: (value: unknown) => void
  reject: (err: Error) => void
  timer: number
}

const RECONNECT_BASE = 1000
const RECONNECT_MAX = 30_000
const REQUEST_TIMEOUT = 15_000
const MAX_QUEUE = 50

function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss://' : 'ws://'
  return `${proto}${location.host}/api/ws`
}

export class RealtimeClient {
  private ws: WebSocket | null = null
  private status: RealtimeStatus = 'disconnected'
  private handlers = new Map<string, Set<MessageHandler>>()
  private statusListeners = new Set<StatusListener>()
  private pending = new Map<string, PendingRequest>()
  private queue: RealtimeEnvelope[] = []
  private seq = 0
  private reconnectDelay = RECONNECT_BASE
  private reconnectTimer: number | null = null
  private manualClose = true
  private disposed = false

  constructor() {
    // Reconnect promptly when the tab becomes visible again (e.g. after a
    // laptop lid / phone lock), instead of waiting out the backoff.
    document.addEventListener('visibilitychange', this.onVisibility)
  }

  private onVisibility = () => {
    if (document.visibilityState === 'visible' && !this.manualClose && this.status === 'disconnected') {
      this.cancelReconnect()
      this.connect()
    }
  }

  getStatus(): RealtimeStatus {
    return this.status
  }

  isConnected(): boolean {
    return this.status === 'connected'
  }

  /** Registers a push handler for a message type. Returns an unsubscribe. */
  on(type: string, handler: MessageHandler): () => void {
    let set = this.handlers.get(type)
    if (!set) {
      set = new Set()
      this.handlers.set(type, set)
    }
    set.add(handler)
    return () => {
      set!.delete(handler)
    }
  }

  /** Subscribes to connection state changes. Returns an unsubscribe. */
  onStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener)
    return () => {
      this.statusListeners.delete(listener)
    }
  }

  /** Opens (or re-opens) the connection. Idempotent while already connected. */
  connect(): void {
    this.manualClose = false
    if (this.ws || this.status === 'connecting') return
    this.setStatus('connecting')
    const ws = new WebSocket(wsURL())
    this.ws = ws
    ws.onopen = () => {
      if (this.ws !== ws) return
      this.reconnectDelay = RECONNECT_BASE
      this.setStatus('connected')
      this.flushQueue()
    }
    ws.onmessage = (ev) => this.handleMessage(ev.data)
    ws.onclose = () => {
      if (this.ws !== ws) return
      this.ws = null
      this.rejectPending(new RealtimeError('disconnected', '连接已断开'))
      if (!this.manualClose) {
        this.setStatus('disconnected')
        this.scheduleReconnect()
      } else {
        this.setStatus('disconnected')
      }
    }
    ws.onerror = () => {
      // onclose follows; nothing to do here.
    }
  }

  /** Closes the connection and stops auto-reconnect (e.g. on logout). */
  disconnect(): void {
    this.manualClose = true
    this.cancelReconnect()
    this.queue = []
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
      this.ws = null
    }
    this.rejectPending(new RealtimeError('disconnected', '连接已关闭'))
    this.setStatus('disconnected')
  }

  /** Sends a fire-and-forget notification; queued while disconnected. */
  send(type: string, data?: unknown): void {
    this.enqueue({ type, data })
  }

  /**
   * Sends an RPC request and resolves with the "<type>.result" payload, or
   * rejects with RealtimeError on "<type>.error" / timeout / disconnect.
   */
  request<T>(type: string, data?: unknown, timeoutMs = REQUEST_TIMEOUT): Promise<T> {
    const id = `req-${++this.seq}-${Date.now()}`
    return new Promise<T>((resolve, reject) => {
      const timer = window.setTimeout(() => {
        this.pending.delete(id)
        reject(new RealtimeError('timeout', `请求超时: ${type}`))
      }, timeoutMs)
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer })
      this.enqueue({ id, type, data })
    })
  }

  private enqueue(env: RealtimeEnvelope): void {
    if (this.status === 'connected' && this.ws) {
      this.ws.send(JSON.stringify(env))
      return
    }
    if (this.queue.length < MAX_QUEUE) {
      this.queue.push(env)
    }
  }

  private flushQueue(): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    for (const env of this.queue) {
      this.ws.send(JSON.stringify(env))
    }
    this.queue = []
  }

  private handleMessage(raw: string): void {
    let env: RealtimeEnvelope
    try {
      env = JSON.parse(raw) as RealtimeEnvelope
    } catch {
      return
    }
    // Correlate RPC responses by id before routing to push handlers.
    if (env.id && this.pending.has(env.id)) {
      const req = this.pending.get(env.id)!
      this.pending.delete(env.id)
      window.clearTimeout(req.timer)
      if (env.type.endsWith('.error')) {
        const err = (env.data ?? {}) as { code?: string; message?: string }
        req.reject(new RealtimeError(err.code ?? 'error', err.message ?? '请求失败'))
      } else {
        req.resolve(env.data)
      }
      return
    }
    const set = this.handlers.get(env.type)
    if (!set) return
    for (const handler of set) {
      handler(env.data)
    }
  }

  private scheduleReconnect(): void {
    if (this.manualClose || this.disposed || this.reconnectTimer !== null) return
    const delay = this.reconnectDelay + Math.random() * 500
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, RECONNECT_MAX)
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      this.connect()
    }, delay)
  }

  private cancelReconnect(): void {
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private rejectPending(err: Error): void {
    for (const [, req] of this.pending) {
      window.clearTimeout(req.timer)
      req.reject(err)
    }
    this.pending.clear()
  }

  private setStatus(status: RealtimeStatus): void {
    if (this.status === status) return
    this.status = status
    for (const listener of this.statusListeners) {
      listener(status)
    }
  }
}

export const realtime = new RealtimeClient()