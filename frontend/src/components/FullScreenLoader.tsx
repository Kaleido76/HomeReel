import { Loader2 } from 'lucide-react'

export function FullScreenLoader() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-50">
      <Loader2 className="size-6 animate-spin text-blue-600" />
    </div>
  )
}
