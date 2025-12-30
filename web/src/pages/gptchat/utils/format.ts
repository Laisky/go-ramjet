/**
 * Utility functions for formatting chat message data.
 */

/**
 * Safely formats costUsd to a fixed decimal string.
 * Handles both number and string types for backward compatibility with stored data.
 *
 * @param costUsd - The cost value which may be a number or string
 * @returns Formatted cost string or null if the value is invalid
 */
export function formatCostUsd(costUsd: unknown): string | null {
  if (costUsd === undefined || costUsd === null || costUsd === '') {
    return null
  }

  const numValue = typeof costUsd === 'number' ? costUsd : Number(costUsd)

  if (Number.isNaN(numValue)) {
    console.debug('[formatCostUsd] Invalid costUsd value:', costUsd)
    return null
  }

  return numValue.toFixed(4)
}
