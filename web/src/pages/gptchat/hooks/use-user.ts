import useSWR from 'swr'
import type { UserConfig } from '../types'
import { api } from '../utils/api'

export function useUser(token: string) {
  const {
    data: user,
    error,
    mutate,
  } = useSWR<UserConfig>(
    token && token !== 'DEFAULT_PROXY_TOKEN' && !token.startsWith('FREETIER-')
      ? ['/user/me', token]
      : null,
    ([, t]: [string, string]) => api.fetchCurrentUser(t),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  )

  return {
    user,
    isLoading: !error && !user && !!token && !token.startsWith('FREETIER-'),
    isError: error,
    mutate,
  }
}
