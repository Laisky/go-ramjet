import useSWR from 'swr'
import { api } from '../utils/api'
import type { UserConfig } from '../types'

export function useUser(token: string) {
  const { data: user, error, mutate } = useSWR<UserConfig>(
    token && token !== 'DEFAULT_PROXY_TOKEN' && !token.startsWith('FREETIER-')
      ? ['/user/me', token]
      : null,
    ([_, t]: [string, string]) => api.fetchCurrentUser(t),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    }
  )

  return {
    user,
    isLoading: !error && !user && !!token && !token.startsWith('FREETIER-'),
    isError: error,
    mutate,
  }
}
