import { act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { buildHeaders } from '@/utils/api'
import {
  installStore,
  PROVIDER_A,
  PROVIDER_B,
  renderLoadedConfig,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

/** ATTACKER_BASE is the endpoint a crafted link tries to switch to. */
const ATTACKER_BASE = 'https://evil.example'

/** OWN_KEY is a synthetic key of the user's own (BYOK shape). */
const OWN_KEY = 'sk-SYNTHETIC-OWN-KEY-0123456789'

/**
 * outboundHeaders returns the headers the chat client sends for config, the
 * way chat-streaming builds them for every request.
 */
async function outboundHeaders(config: {
  api_token: string
  api_base: string
}) {
  return buildHeaders(
    config.api_token,
    config.api_base !== 'https://api.openai.com' ? config.api_base : undefined,
  )
}

/**
 * ownKeyStore seeds session 1 with the user's own key on PROVIDER_A and
 * returns the backing store.
 */
function ownKeyStore(token = OWN_KEY): Record<string, unknown> {
  return installStore({
    config_selected_session: 1,
    config_sync_key: SYNC_KEY,
    chat_user_config_1: sessionConfig({
      api_base: PROVIDER_A,
      api_token: token,
    }),
  })
}

describe('useConfig link-supplied API endpoint', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
    vi.restoreAllMocks()
  })

  it('never re-targets a stored key of the user’s own at a link-supplied endpoint', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()

    // The server forwards Authorization to X-Laisky-Api-Base for BYOK keys,
    // so these two headers together decide where the stored key goes.
    const headers = await outboundHeaders(result.current.config)
    expect(headers.Authorization).toBe(`Bearer ${OWN_KEY}`)
    expect(headers['X-Laisky-Api-Base']).toBe(PROVIDER_A)
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    // The link is not silently dropped: the user is asked instead.
    expect(result.current.pendingApiBase).toEqual({
      sessionId: 1,
      apiBase: ATTACKER_BASE,
    })
    expect(window.location.search).toBe('')
  })

  it('treats a custom-shaped key as the user’s own too', async () => {
    window.history.replaceState({}, '', `/?base=${ATTACKER_BASE}`)
    const store = ownKeyStore('azure-SYNTHETIC-custom-key-0123')

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.pendingApiBase?.apiBase).toBe(ATTACKER_BASE)
  })

  it('applies the endpoint once the user confirms it', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.confirmPendingApiBase()
    })

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingApiBase).toBeNull()
  })

  it('keeps the current endpoint when the user declines', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    act(() => {
      result.current.dismissPendingApiBase()
    })

    expect(result.current.pendingApiBase).toBeNull()
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
  })

  it('never applies a pending endpoint to a session the user switched to', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: OWN_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-SYNTHETIC-OTHER-KEY-0123456789',
        session_name: 'Second',
      }),
    })

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.switchSession(2)
    })
    expect(result.current.pendingApiBase).toBeNull()
    await act(async () => {
      await result.current.confirmPendingApiBase()
    })

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 2).api_base).toBe(PROVIDER_A)
  })

  it('applies an endpoint that comes with its own key in the same link', async () => {
    window.history.replaceState(
      {},
      '',
      `/?api_key=sk-SYNTHETIC-LINK-KEY-0123456789&api_base=${PROVIDER_B}`,
    )
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(
      'sk-SYNTHETIC-LINK-KEY-0123456789',
    )
    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingApiBase).toBeNull()
  })

  it('still asks when the link key is rejected and the stored key would move', async () => {
    window.history.replaceState(
      {},
      '',
      `/?api_key=${SYNC_KEY}&api_base=${ATTACKER_BASE}`,
    )
    ownKeyStore()

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(result.current.pendingApiBase?.apiBase).toBe(ATTACKER_BASE)
  })

  it('applies a link endpoint to a free-tier session without asking', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
      }),
    })

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingApiBase).toBeNull()
  })

  it('applies a link endpoint on a first visit without asking', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store = installStore({})

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingApiBase).toBeNull()
  })

  it('does not ask when the link names the stored endpoint', async () => {
    window.history.replaceState(
      {},
      '',
      `/?api_base=${PROVIDER_A.toUpperCase()}/`,
    )
    ownKeyStore()

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(result.current.pendingApiBase).toBeNull()
  })
})
