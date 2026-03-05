import useSWR from 'swr'
import type { UserConfig } from '../types'
import { api } from '../utils/api'

export function useUser(token: string) {
  const {
    data: user,
    error,
    mutate,
  } = useSWR<UserConfig>(
    token ? ['/user/me', token] : null,
    ([, t]: [string, string]) => api.fetchCurrentUser(t),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  )

  return {
    user,
    isLoading: !error && !user && !!token,
    isError: error,
    mutate,
  }
}
