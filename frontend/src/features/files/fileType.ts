import { File, FileArchive, FileCode, FileText, FileVideo, Image, Music, type LucideIcon } from 'lucide-react'

// File-kind detection for the file browser. Kinds map to muted, subtly distinct
// icon + color pairs — distinguishable at a glance without turning the list
// into a rainbow.

type FileKind = 'video' | 'audio' | 'image' | 'archive' | 'doc' | 'code' | 'file'

const KIND_EXTS: Record<Exclude<FileKind, 'file'>, string[]> = {
  video: ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'mpg', 'mpeg', 'ts', 'm2ts', '3gp', 'ogv', 'rmvb', 'rm'],
  audio: ['mp3', 'wav', 'flac', 'aac', 'ogg', 'oga', 'm4a', 'wma', 'opus', 'aiff', 'ape', 'amr'],
  image: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg', 'ico', 'tif', 'tiff', 'heic', 'avif', 'jfif'],
  archive: ['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'iso', 'lz4', 'zst'],
  doc: ['txt', 'md', 'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'rtf', 'epub', 'csv'],
  code: ['js', 'ts', 'jsx', 'tsx', 'go', 'py', 'java', 'c', 'cpp', 'h', 'hpp', 'html', 'htm', 'css', 'json', 'yaml', 'yml', 'xml', 'sh', 'bat', 'ps1', 'sql'],
}

const EXT_TO_KIND: Record<string, FileKind> = {}
for (const [kind, exts] of Object.entries(KIND_EXTS)) {
  for (const ext of exts) EXT_TO_KIND[ext] = kind as FileKind
}

const KIND_STYLE: Record<FileKind, { icon: LucideIcon; className: string }> = {
  video: { icon: FileVideo, className: 'text-blue-600' },
  audio: { icon: Music, className: 'text-purple-500' },
  image: { icon: Image, className: 'text-emerald-600' },
  archive: { icon: FileArchive, className: 'text-amber-600' },
  doc: { icon: FileText, className: 'text-neutral-500' },
  code: { icon: FileCode, className: 'text-teal-600' },
  file: { icon: File, className: 'text-neutral-400' },
}

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i > 0 ? name.slice(i + 1).toLowerCase() : ''
}

// fileStyle returns the muted icon + color pair for a file name.
export function fileStyle(name: string): { icon: LucideIcon; className: string } {
  return KIND_STYLE[EXT_TO_KIND[extOf(name)] ?? 'file']
}

const mediaExts = new Set([...KIND_EXTS.video, ...KIND_EXTS.audio])

// isMediaName reports whether the file name has a video or audio extension
// (used by the toolbar's multimedia view filter).
export function isMediaName(name: string): boolean {
  return mediaExts.has(extOf(name))
}
