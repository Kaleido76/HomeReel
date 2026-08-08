import type { ReactNode } from 'react'

// ToolDrawerShell is the generic bottom tool drawer for the file view. It sits
// as the last flex child of the files column (same width as the list, never
// overlapping the left rail), and its open state animates a slide-up that
// pushes the file list above it. Content is drawer-specific and fully owned by
// the caller, so future tool drawers can reuse the shell unchanged.
export function ToolDrawerShell({ open, children }: { open: boolean; children: ReactNode }) {
  return (
    <div
      className={`shrink-0 overflow-hidden bg-white transition-[max-height] duration-300 ease-in-out ${
        open ? 'max-h-60 border-t border-neutral-200 shadow-[0_-4px_12px_rgba(0,0,0,0.05)]' : 'max-h-0'
      }`}
    >
      {children}
    </div>
  )
}
