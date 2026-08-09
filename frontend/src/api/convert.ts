import { api } from './client'
import type { ConvertParams } from '../features/tools/format/presets'
import { DEFAULT_PARAMS } from '../features/tools/format/presets'

// ConvertJob is one enqueued format-factory conversion unit. A video file is
// turned into an mp4 copy next to it; a directory becomes a sibling " (MP4)"
// folder holding the converted copies of its direct-level videos.
export interface ConvertJob {
  path: string
  kind: 'file' | 'dir'
  job_id: string
  output: string
}

export interface ConvertError {
  path: string
  message: string
}

export interface ConvertResult {
  jobs: ConvertJob[]
  errors: ConvertError[]
}

// convertPaths enqueues conversions for the given absolute paths using the
// operations-panel parameters. Invalid selections are reported per-path.
export function convertPaths(paths: string[], params: ConvertParams = DEFAULT_PARAMS): Promise<ConvertResult> {
  return api<ConvertResult>('/api/convert', {
    method: 'POST',
    body: JSON.stringify({ paths, params }),
  })
}

// ConvertProbe is one probed video's stream facts (codecs + duration), used by
// the operations panel to guide the user and disable irrelevant options.
export interface ConvertProbe {
  path: string
  kind: 'file' | 'dir'
  video_codec: string
  audio_codecs: string[]
  subtitle_codecs: string[]
  duration: number
  has_bitmap_subtitle: boolean
}

// probePaths probes the selected paths (directories expand to their direct-level
// videos) so the panel can show what each file contains.
export function probePaths(paths: string[]): Promise<{ results: ConvertProbe[] }> {
  return api<{ results: ConvertProbe[] }>('/api/convert/probe', {
    method: 'POST',
    body: JSON.stringify({ paths }),
  })
}
