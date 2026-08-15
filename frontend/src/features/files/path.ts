// Path helpers for the generic file browser. Paths are absolute host paths in
// the native style (Windows drive letters with backslashes, unix "/").

export interface Crumb {
  label: string
  path: string
}

// pathSegments splits an absolute path into clickable breadcrumb segments. Each
// segment carries the full path up to that point, so clicking it navigates to
// that ancestor. Windows paths yield the drive root (e.g. "C:\") as the first
// segment; unix paths start at "/".
export function pathSegments(p: string): Crumb[] {
  if (!p) return []
  const drive = /^[A-Za-z]:\\?/.exec(p)?.[0]
  if (drive) {
    const rest = p.slice(drive.length).replace(/[\\/]+/g, '/').replace(/\/+$/, '')
    const crumbs: Crumb[] = [{ label: drive, path: drive }]
    if (rest) {
      let acc = drive
      for (const part of rest.split('/')) {
        acc += part
        crumbs.push({ label: part, path: acc })
        acc += '\\'
      }
    }
    return crumbs
  }
  if (p.startsWith('/')) {
    const parts = p.split('/').filter(Boolean)
    const crumbs: Crumb[] = [{ label: '/', path: '/' }]
    let acc = ''
    for (const part of parts) {
      acc += '/' + part
      crumbs.push({ label: part, path: acc })
    }
    return crumbs
  }
  return [{ label: p, path: p }]
}

// basename returns the last path segment of a native path (file name, or folder
// name when p is a directory). The split handles both backslash and slash
// separators; a trailing separator yields "" exactly like the former
// `p.split(/[\\/]/).pop()` the call sites used. split never returns an empty
// array, so no fallback is needed.
export function basename(p: string): string {
  const parts = p.split(/[\\/]/)
  return parts[parts.length - 1]
}

// parentPath returns the parent directory of an absolute path, or null when the
// path is already a root (drive root on Windows, "/" on unix).
export function parentPath(p: string): string | null {
  if (!p) return null
  if (/^[A-Za-z]:\\?$/.test(p)) return null
  const cleaned = p.replace(/[\\/]+$/, '')
  const sep = cleaned.includes('\\') ? '\\' : '/'
  const idx = cleaned.lastIndexOf(sep)
  if (idx <= 0) {
    if (sep === '/' && idx === 0) return '/'
    return null
  }
  const parent = cleaned.slice(0, idx)
  if (/^[A-Za-z]:$/.test(parent)) return parent + '\\'
  return parent
}

// underDrive reports whether the current path belongs to a given drive root.
export function underDrive(path: string, driveRoot: string): boolean {
  if (!path) return false
  if (!driveRoot) return false
  if (path === driveRoot) return true
  const base = driveRoot.replace(/\\$/, '')
  return path.startsWith(base + '\\')
}

export function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`
}

export function formatTime(sec: number): string {
  if (!sec) return ''
  return new Date(sec * 1000).toLocaleString()
}
