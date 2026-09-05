import { ArrowLeft } from 'lucide-react'

// NarrowBack is the single-column back link shown on narrow screens where a
// multi-panel layout collapses to a single full-page column per level. Used by
// library (stack navigation) and file page (drawer detail views).
export function NarrowBack({ label, onBack }: { label: string; onBack: () => void }) {
  return (
    <div className="flex items-center gap-2 border-b border-neutral-200 bg-white px-4 py-2">
      <button
        onClick={onBack}
        className="flex items-center gap-1.5 rounded px-2 py-1 text-sm text-neutral-600 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
      >
        <ArrowLeft className="size-4" /> {label}
      </button>
    </div>
  )
}
