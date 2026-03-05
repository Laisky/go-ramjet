import { useMemo, useRef } from 'react'

/**
 * ChatStorageConcurrencyState encapsulates load and mutation counters for chat storage operations.
 */
export interface ChatStorageConcurrencyState {
  beginLoad: (
    sessionId: number,
  ) => { loadToken: number; startMutationVersion: number }
  markMutation: (sessionId: number) => void
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
  const mutationVersionBySessionRef = useRef(new Map<number, number>())

  /**
   * getMutationVersionForSession returns the current mutation version for a given session.
   *
   * @param sessionId - The chat session identifier.
   * @returns The current mutation version for the session.
   */
  const getMutationVersionForSession = (sessionId: number): number =>
    mutationVersionBySessionRef.current.get(sessionId) || 0

  return useMemo(
    () => ({
      /**
       * beginLoad marks a new load cycle and captures its starting mutation version.
       *
       * @returns The new load token and mutation version snapshot for this load.
       */
      beginLoad: (sessionId: number) => ({
        loadToken: ++loadTokenRef.current,
        startMutationVersion: getMutationVersionForSession(sessionId),
      }),

      /**
       * markMutation increments mutation version so in-flight loads can be invalidated.
       */
      markMutation: (sessionId: number) => {
        const currentVersion = getMutationVersionForSession(sessionId)
        mutationVersionBySessionRef.current.set(sessionId, currentVersion + 1)
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
        getMutationVersionForSession(loadingSessionId) !== startMutationVersion,

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
      getMutationVersion: () => {
        let highest = 0
        for (const version of mutationVersionBySessionRef.current.values()) {
          if (version > highest) {
            highest = version
          }
        }

        return highest
      },
    }),
    [],
  )
}
