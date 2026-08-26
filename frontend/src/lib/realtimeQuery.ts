import type { QueryClient, QueryKey } from '@tanstack/react-query'
import type { RealtimeClient } from '../api/realtime'

// invalidateOnMessage wires server pushes to the TanStack Query cache: whenever
// a message of the mapped type arrives, the listed query keys are invalidated
// so subscribed queries refetch the latest state. This is the primary
// migration path from polling — a feature only declares "收到 X → 失效 Y".
//
// Returns an unsubscribe to be called on unmount.
export function invalidateOnMessage(
  client: RealtimeClient,
  queryClient: QueryClient,
  mapping: Record<string, QueryKey[]>,
): () => void {
  const unsubscribers = Object.entries(mapping).map(([type, keys]) =>
    client.on(type, () => {
      for (const key of keys) {
        void queryClient.invalidateQueries({ queryKey: key })
      }
    }),
  )
  return () => {
    for (const unsubscribe of unsubscribers) unsubscribe()
  }
}