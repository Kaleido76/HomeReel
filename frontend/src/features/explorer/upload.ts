import { uploadChunk } from '../../api/storages'

const CHUNK_SIZE = 8 * 1024 * 1024
const CHUNK_RETRIES = 3

// uploadChunked streams a File into the chunked upload endpoint, retrying
// transient connection failures. Chunks are idempotent on the server (parts are
// overwritten), so retrying is safe; the backend tolerates a retried final
// chunk after a lost response. Reports 0..1 progress via onProgress.
export async function uploadChunked(
  storageId: string,
  path: string,
  file: File,
  onProgress?: (ratio: number) => void,
): Promise<void> {
  const uploadId = newUploadId()
  const total = Math.max(1, Math.ceil(file.size / CHUNK_SIZE))
  for (let i = 0; i < total; i++) {
    const blob = file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE)
    await uploadChunkWithRetry(storageId, path, uploadId, file.name, i, total, blob)
    onProgress?.((i + 1) / total)
  }
}

async function uploadChunkWithRetry(
  storageId: string,
  path: string,
  uploadId: string,
  filename: string,
  index: number,
  total: number,
  blob: Blob,
): Promise<void> {
  let lastErr: unknown
  for (let attempt = 0; attempt < CHUNK_RETRIES; attempt++) {
    try {
      await uploadChunk(storageId, path, uploadId, filename, index, total, blob)
      return
    } catch (err) {
      lastErr = err
      if (attempt < CHUNK_RETRIES - 1) {
        await new Promise((r) => setTimeout(r, 500 * (attempt + 1)))
      }
    }
  }
  throw lastErr
}

function newUploadId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `u-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}
