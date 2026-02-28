import { useMemo, useRef } from 'react'

/**
 * ChatStorageConcurrencyState encapsulates load and mutation counters for chat storage operations.
 */
export interface ChatStorageConcurrencyState {
  beginLoad: () => { loadToken: number; startMutationVersion: number }
  markMutation: () => void
  isStaleLoad: (params: {
    loadingSessionId: number
    currentSessionId: number
    loadToken: number
    startMutationVersion: number
  }) => boolean
  getLoadToken: () => number
  getMutationVersion: () => number
}

/**
 * useChatStorageConcurrencyState creates concurrency guards for chat loading and mutations.
 *
 * It tracks a monotonic load token and mutation version to invalidate stale in-flight loads.
 *
 * @returns A state object with helpers to begin loads, mark mutations, and check staleness.
 */
export function useChatStorageConcurrencyState(): ChatStorageConcurrencyState {
  const loadTokenRef = useRef(0)
  const mutationVersionRef = useRef(0)

  return useMemo(
    () => ({
      /**
       * beginLoad marks a new load cycle and captures its starting mutation version.
       *
       * @returns The new load token and mutation version snapshot for this load.
       */
      beginLoad: () => ({
        loadToken: ++loadTokenRef.current,
        startMutationVersion: mutationVersionRef.current,
      }),

      /**
       * markMutation increments mutation version so in-flight loads can be invalidated.
       */
      markMutation: () => {
        mutationVersionRef.current += 1
      },

      /**
       * isStaleLoad checks whether a load is stale due to session switch, newer load, or mutation.
       *
       * @param params - Identifiers for the in-flight load and current runtime state.
       * @returns True when the in-flight load should be dropped.
       */
      isStaleLoad: ({
        loadingSessionId,
        currentSessionId,
        loadToken,
        startMutationVersion,
      }: {
        loadingSessionId: number
        currentSessionId: number
        loadToken: number
        startMutationVersion: number
      }) =>
        loadingSessionId !== currentSessionId ||
        loadToken !== loadTokenRef.current ||
        mutationVersionRef.current !== startMutationVersion,

      /**
       * getLoadToken returns the latest load token.
       *
       * @returns Current load token value.
       */
      getLoadToken: () => loadTokenRef.current,

      /**
       * getMutationVersion returns the latest mutation version.
       *
       * @returns Current mutation version value.
       */
      getMutationVersion: () => mutationVersionRef.current,
    }),
    [],
  )
}
