// ProgressBar is the shared thin progress indicator (a neutral track with a blue
// fill) used by the playback sections and series member rows. value is a
// percentage clamped to 0..100; className carries the size/margin of the track
// (e.g. "mt-2 h-1.5 w-full").
export function ProgressBar({ value, className = '' }: { value: number; className?: string }) {
  return (
    <div className={`overflow-hidden rounded-sm bg-neutral-100 ${className}`}>
      <div className="h-full rounded-sm bg-blue-600" style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
    </div>
  )
}
