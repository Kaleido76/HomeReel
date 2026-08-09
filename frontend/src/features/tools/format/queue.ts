import { useSyncExternalStore } from 'react'

// ConvertTarget is one file/folder handed over from the 文件 tab to the
// 格式工厂 tab. It lives in a module store (not the URL) because a selection
// can be dozens of long absolute paths and is transient anyway — refreshing
// the format tab simply shows the job queue without a pending batch.
export interface ConvertTarget {
  path: string
  name: string
  is_dir: boolean
}

let pending: ConvertTarget[] = []
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((l) => l())
}

export function setPending(items: ConvertTarget[]) {
  pending = items
  emit()
}

export function getPending(): ConvertTarget[] {
  return pending
}

export function clearPending() {
  setPending([])
}

// usePending subscribes to the pending conversion batch (empty when none).
export function usePending(): ConvertTarget[] {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    getPending,
  )
}

// intendedOutput mirrors the backend's naming: a file becomes same-name.mp4
// next to it, a directory becomes a sibling " (MP4)" folder. The actual output
// may still bump to a " (N)" suffix at run time on a name collision; a source
// that is already .mp4 is always shown as " (1)" because it can never share
// its own path.
export function intendedOutput(target: ConvertTarget): string {
  if (target.is_dir) {
    const parent = target.path.slice(0, target.path.length - target.name.length)
    return `${parent}${target.name} (MP4)`
  }
  const dot = target.name.lastIndexOf('.')
  const stem = dot > 0 ? target.name.slice(0, dot) : target.name
  const parent = target.path.slice(0, target.path.length - target.name.length)
  const out = `${parent}${stem}.mp4`
  return out === target.path ? `${parent}${stem} (1).mp4` : out
}
