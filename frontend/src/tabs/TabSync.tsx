import { useEffect } from 'react'
import { initTabSync } from './manager'

// TabSync keeps the browser URL in sync with the active tab's memory history
// (push on in-tab navigation, realign on back/forward). It renders nothing.
export function TabSync() {
  useEffect(() => initTabSync(), [])
  return null
}
