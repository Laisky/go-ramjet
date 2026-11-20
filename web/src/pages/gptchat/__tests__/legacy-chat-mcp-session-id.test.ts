import { webcrypto } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { loadLegacyChatDeclarations } from './fixtures/legacy-chat-script'

type UuidApi = { generateUUIDv4: () => string }

const UUID_V4 =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

/**
 * loadGenerator evaluates generateUUIDv4 with the given `crypto` binding, which
 * lets a test model browsers where crypto.randomUUID is unavailable (it is
 * exposed only in secure contexts) while crypto.getRandomValues still exists.
 *
 * @param cryptoBinding - The value the legacy code sees as `crypto`.
 * @returns The evaluated generateUUIDv4 function.
 */
function loadGenerator(cryptoBinding: unknown): UuidApi {
  return loadLegacyChatDeclarations<UuidApi>(['generateUUIDv4'], {
    crypto: cryptoBinding,
  })
}

describe('legacy chat.js generateUUIDv4 MCP session ids (alerts #84, #85)', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('prefers crypto.randomUUID when the context exposes it', () => {
    const randomUUID = vi.fn(() => '3b241101-e2bb-4255-8caf-4136c566a962')
    const { generateUUIDv4 } = loadGenerator({ randomUUID })

    expect(generateUUIDv4()).toBe('3b241101-e2bb-4255-8caf-4136c566a962')
    expect(randomUUID).toHaveBeenCalledTimes(1)
  })

  it('uses crypto.getRandomValues, not Math.random, without randomUUID', () => {
    const mathRandom = vi.spyOn(Math, 'random')
    const getRandomValues = vi.fn(
      <T extends ArrayBufferView>(array: T) =>
        webcrypto.getRandomValues(
          array as unknown as Uint8Array,
        ) as unknown as T,
    )
    const { generateUUIDv4 } = loadGenerator({ getRandomValues })

    const ids = new Set(Array.from({ length: 256 }, () => generateUUIDv4()))

    expect(mathRandom).not.toHaveBeenCalled()
    expect(getRandomValues).toHaveBeenCalled()
    expect(ids.size).toBe(256)
    ids.forEach((id) => expect(id).toMatch(UUID_V4))
  })

  it('sets the RFC 9562 version and variant bits on the fallback', () => {
    const { generateUUIDv4 } = loadGenerator({
      getRandomValues: (array: Uint8Array) => array.fill(0xff),
    })

    expect(generateUUIDv4()).toBe('ffffffff-ffff-4fff-bfff-ffffffffffff')
  })

  it('refuses to fall back to Math.random when no CSPRNG exists', () => {
    const mathRandom = vi.spyOn(Math, 'random')
    const { generateUUIDv4 } = loadGenerator(undefined)

    expect(() => generateUUIDv4()).toThrow(/secure random/i)
    expect(mathRandom).not.toHaveBeenCalled()
  })
})
