import { kvGet } from '@/utils/storage'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_SESSION_ID, getActiveSessionId } from '../config-helpers'

// Mock storage
vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  StorageKeys: {
    SELECTED_SESSION: 'config_selected_session',
  },
}))

describe('config-helpers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getActiveSessionId', () => {
    it('should return DEFAULT_SESSION_ID if no session is selected', async () => {
      ;(kvGet as any).mockResolvedValue(null)
      const id = await getActiveSessionId()
      expect(id).toBe(DEFAULT_SESSION_ID)
    })

    it('should return the selected session ID from storage', async () => {
      ;(kvGet as any).mockResolvedValue(2)
      const id = await getActiveSessionId()
      expect(id).toBe(2)
    })

    it('should handle string session IDs from storage', async () => {
      ;(kvGet as any).mockResolvedValue('3')
      const id = await getActiveSessionId()
      expect(id).toBe(3)
      expect(typeof id).toBe('number')
    })

    it('should return DEFAULT_SESSION_ID if storage contains invalid value', async () => {
      ;(kvGet as any).mockResolvedValue('abc')
      const id = await getActiveSessionId()
      expect(id).toBe(DEFAULT_SESSION_ID)
    })
  })
})
