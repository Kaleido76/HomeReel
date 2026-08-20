// DevLog is the frontend log capture behind the 开发者工具 (dev tools) tool. It
// exists so a device without an openable dev console (e.g. mobile) can still
// record what the frontend printed. Two sources feed it:
//   - a console.* hook, installed while capture is enabled (covers third-party
//     output too), and
//   - the explicit `devLog(module, level, ...)` helper, which tags a line with a
//     module name so the viewer can filter by it (Android-Studio style).
//
// Captured lines live in a ring buffer (DEVLOG_LIMIT) so memory stays bounded;
// the 开发者工具 page shows them live and can clear or archive them.

export type DevLogLevel = 'log' | 'info' | 'warn' | 'error' | 'debug'

export interface DevLogLine {
  timestamp: string
  level: DevLogLevel
  module: string
  message: string
}

const DEVLOG_LIMIT = 2000
const ENABLED_KEY = 'devtools.enabled'

type Listener = () => void

let enabled = localStorage.getItem(ENABLED_KEY) === '1'
let lines: DevLogLine[] = []
const listeners = new Set<Listener>()

// consoleOut holds the original console methods captured at module load, so
// devLog() can print to the real browser console without being re-captured by
// the hook below (which would double-push the same line).
const consoleOut = {
  log: console.log.bind(console),
  info: console.info.bind(console),
  warn: console.warn.bind(console),
  error: console.error.bind(console),
  debug: console.debug.bind(console),
}

let installed = false
const installedMethods = {
  log: consoleOut.log,
  info: consoleOut.info,
  warn: consoleOut.warn,
  error: consoleOut.error,
  debug: consoleOut.debug,
}

// formatArgs joins console-style arguments into one searchable string, keeping
// JSON-ish representation for objects.
function formatArgs(args: unknown[]): string {
  return args
    .map((a) => {
      if (typeof a === 'string') return a
      if (a instanceof Error) return a.stack ?? a.message
      try {
        return JSON.stringify(a)
      } catch {
        return String(a)
      }
    })
    .join(' ')
}

function push(level: DevLogLevel, module: string, args: unknown[]) {
  if (!enabled) return
  lines.push({
    timestamp: new Date().toISOString(),
    level,
    module,
    message: formatArgs(args),
  })
  if (lines.length > DEVLOG_LIMIT) {
    lines.splice(0, lines.length - DEVLOG_LIMIT)
  }
  for (const l of listeners) l()
}

// install/restore hook window.console while capture is on, forwarding real
// output to the browser console AND into the ring buffer.
function installConsoleHook() {
  if (installed || typeof window === 'undefined') return
  const wrap = (level: DevLogLevel, real: (...a: unknown[]) => void) =>
    (...args: unknown[]) => {
      real(...args)
      push(level, 'console', args)
    }
  console.log = wrap('log', consoleOut.log) as typeof console.log
  console.info = wrap('info', consoleOut.info) as typeof console.info
  console.warn = wrap('warn', consoleOut.warn) as typeof console.warn
  console.error = wrap('error', consoleOut.error) as typeof console.error
  console.debug = wrap('debug', consoleOut.debug) as typeof console.debug
  installed = true
}

function restoreConsoleHook() {
  if (!installed) return
  console.log = installedMethods.log
  console.info = installedMethods.info
  console.warn = installedMethods.warn
  console.error = installedMethods.error
  console.debug = installedMethods.debug
  installed = false
}

export function setDevLogEnabled(v: boolean) {
  enabled = v
  localStorage.setItem(ENABLED_KEY, v ? '1' : '0')
  if (v) installConsoleHook()
  else restoreConsoleHook()
  for (const l of listeners) l()
}

export function isDevLogEnabled(): boolean {
  return enabled
}

// initDevLog installs the console hook up-front if capture was left enabled, so
// logging works from app startup even before the 开发者工具 page is ever opened.
export function initDevLog() {
  if (enabled) installConsoleHook()
}

export function subscribeDevLog(fn: Listener): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

// devLog is the module-tagged logging API business code can call so the 开发者
// 工具 viewer can filter by module. It forwards to the real (unhooked) console
// methods so the line prints to the browser console exactly once, then records
// it into the ring buffer tagged with the module.
export function devLog(module: string, level: DevLogLevel, ...args: unknown[]) {
  if (!enabled) return
  if (level === 'warn') consoleOut.warn(...args)
  else if (level === 'error') consoleOut.error(...args)
  else consoleOut.log(...args)
  push(level, module, args)
}

export function getDevLogLines(): DevLogLine[] {
  return lines
}

export function clearDevLog() {
  lines = []
  for (const l of listeners) l()
}
