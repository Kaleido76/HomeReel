import { useEffect, useState } from 'react'

// useMediaQuery subscribes to a CSS media query and returns whether it matches.
// Used to pick between layouts that differ structurally (e.g. Finder columns
// vs. single-column file list) rather than purely via CSS.
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => (typeof window !== 'undefined' ? window.matchMedia(query).matches : false))

  useEffect(() => {
    const mql = window.matchMedia(query)
    const onChange = (e: MediaQueryListEvent) => setMatches(e.matches)
    setMatches(mql.matches)
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [query])

  return matches
}
