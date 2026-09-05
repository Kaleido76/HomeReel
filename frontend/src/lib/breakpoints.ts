import { useMediaQuery } from './useMediaQuery'

export const WIDE = '(min-width: 1024px)'

export function useIsWide(): boolean {
  return useMediaQuery(WIDE)
}
