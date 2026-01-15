import { afterEach, describe, expect, it, vi } from 'vitest'

import { getSHA1 } from '../api'

describe('getSHA1', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('falls back when crypto.subtle is unavailable', async () => {
    const debugSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})

    vi.stubGlobal('crypto', undefined)

    const hash = await getSHA1('test-token')

    expect(hash).toMatch(/^[0-9a-f]{8}$/)
    expect(debugSpy).toHaveBeenCalled()

    debugSpy.mockRestore()
  })
})
