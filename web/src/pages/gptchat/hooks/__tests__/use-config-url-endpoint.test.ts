import { act, renderHook, waitFor } from '@testing-library/react'
import { createElement, StrictMode, type ReactNode } from 'react'
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
import { useConfig } from '../use-config'

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

/** LINK_KEY is a synthetic key that a link carries. */
const LINK_KEY = 'sk-SYNTHETIC-LINK-KEY-0123456789'

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
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: ATTACKER_BASE,
    })
    expect(window.location.search).toBe('')
  })

  it('still asks after a StrictMode double mount', async () => {
    // StrictMode runs the load effect twice. The first, already cleaned-up
    // run consumes the link's parameters, so its prompt must still reach the
    // page that stays mounted.
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = renderHook(() => useConfig(), {
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(StrictMode, null, children),
    })
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    await waitFor(() => {
      expect(result.current.pendingLinkChange).toEqual({
        sessionId: 1,
        apiBase: ATTACKER_BASE,
      })
    })
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(window.location.search).toBe('')
  })

  it('treats a custom-shaped key as the user’s own too', async () => {
    window.history.replaceState({}, '', `/?base=${ATTACKER_BASE}`)
    const store = ownKeyStore('azure-SYNTHETIC-custom-key-0123')

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.pendingLinkChange?.apiBase).toBe(ATTACKER_BASE)
  })

  it('applies the endpoint once the user confirms it', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingLinkChange).toBeNull()
  })

  it('keeps the current endpoint when the user declines', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    act(() => {
      result.current.dismissPendingLinkChange()
    })

    expect(result.current.pendingLinkChange).toBeNull()
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
    expect(result.current.pendingLinkChange).toBeNull()
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 2).api_base).toBe(PROVIDER_A)
  })

  it('applies a link key and endpoint together once the user confirms', async () => {
    window.history.replaceState(
      {},
      '',
      `/?api_key=${LINK_KEY}&api_base=${PROVIDER_B}`,
    )
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: PROVIDER_B,
      apiToken: LINK_KEY,
    })
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(result.current.config.api_token).toBe(LINK_KEY)
    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(storedConfig(store, 1).api_token).toBe(LINK_KEY)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingLinkChange).toBeNull()
  })

  it('asks before a link moves an existing free-tier session to its own key and endpoint', async () => {
    // The server forwards a BYOK key, and the session's recent history with
    // every request, to X-Laisky-Api-Base. A link that silently switched a
    // stored free-tier session would hand that history to the link's host.
    window.history.replaceState(
      {},
      '',
      `/?api_key=${LINK_KEY}&api_base=${ATTACKER_BASE}`,
    )
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
      }),
    })

    const { result } = await renderLoadedConfig()

    const headers = await outboundHeaders(result.current.config)
    expect(headers.Authorization).toBe(
      'Bearer FREETIER-synthetic-placeholder-01',
    )
    expect(headers['X-Laisky-Api-Base']).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_token).toBe(
      'FREETIER-synthetic-placeholder-01',
    )
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: ATTACKER_BASE,
      apiToken: LINK_KEY,
    })
    expect(window.location.search).toBe('')

    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })
    expect(storedConfig(store, 1).api_token).toBe(LINK_KEY)
    expect(storedConfig(store, 1).api_base).toBe(ATTACKER_BASE)
  })

  it('still applies a link key alone to a free-tier session without asking', async () => {
    window.history.replaceState({}, '', `/?api_key=${LINK_KEY}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
      }),
    })

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(LINK_KEY)
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_token).toBe(LINK_KEY)
    expect(result.current.pendingLinkChange).toBeNull()
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
    expect(result.current.pendingLinkChange?.apiBase).toBe(ATTACKER_BASE)
  })

  it('asks before a link moves an existing free-tier session to another endpoint', async () => {
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

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: PROVIDER_B,
    })
  })

  it('asks before a link moves a session awaiting a reset with no key set aside', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
        api_token_reset_required: true,
      }),
    })

    const { result } = await renderLoadedConfig()

    // The key the user types into the reset alert next would otherwise go
    // straight to the link's host.
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.config.api_token_reset_required).toBe(true)
    expect(result.current.pendingLinkChange?.apiBase).toBe(ATTACKER_BASE)
  })

  it('applies a link endpoint on a first visit without asking', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store = installStore({})

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_B)
    expect(result.current.pendingLinkChange).toBeNull()
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
    expect(result.current.pendingLinkChange).toBeNull()
  })
})

describe('useConfig link-supplied API key', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
    vi.restoreAllMocks()
  })

  it.each([
    ['an empty api_key', `/?api_key=&api_base=${ATTACKER_BASE}`],
    ['a junk token', `/?token=x&base=${ATTACKER_BASE}`],
    [
      'a free-tier token',
      `/?api_key=FREETIER-synthetic-0123456789&api_base=${ATTACKER_BASE}`,
    ],
  ])(
    'keeps the stored key and endpoint when a link carries %s',
    async (_name, search) => {
      window.history.replaceState({}, '', search)
      const store = ownKeyStore()

      const { result } = await renderLoadedConfig()

      expect(result.current.config.api_token).toBe(OWN_KEY)
      expect(result.current.config.api_base).toBe(PROVIDER_A)
      expect(storedConfig(store, 1).api_token).toBe(OWN_KEY)
      expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
      // Only the endpoint is offered. An empty, free-tier or implausibly
      // short link key is dropped rather than offered as a replacement.
      expect(result.current.pendingLinkChange).toEqual({
        sessionId: 1,
        apiBase: ATTACKER_BASE,
      })
    },
  )

  it('never silently replaces the user’s own key with a link key', async () => {
    window.history.replaceState(
      {},
      '',
      `/?api_key=${LINK_KEY}&api_base=${ATTACKER_BASE}`,
    )
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_token).toBe(OWN_KEY)
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: ATTACKER_BASE,
      apiToken: LINK_KEY,
    })
  })

  it('asks before a link key alone replaces the user’s own key', async () => {
    window.history.replaceState({}, '', `/?api_key=${LINK_KEY}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(OWN_KEY)
    expect(storedConfig(store, 1).api_token).toBe(OWN_KEY)
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiToken: LINK_KEY,
    })
  })

  it('keeps a key set aside by a pending reset on its endpoint', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
        api_token_reset_required: true,
        api_token_before_reset: OWN_KEY,
      }),
    })

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.config.api_token_before_reset).toBe(OWN_KEY)
    expect(result.current.config.api_token_reset_required).toBe(true)
    expect(result.current.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: ATTACKER_BASE,
    })
  })

  it('completes a pending reset with a confirmed link key', async () => {
    window.history.replaceState({}, '', `/?api_key=${LINK_KEY}`)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'FREETIER-synthetic-placeholder-01',
        api_token_reset_required: true,
        api_token_before_reset: OWN_KEY,
      }),
    })

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token_reset_required).toBe(true)
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(result.current.config.api_token).toBe(LINK_KEY)
    expect(result.current.config.api_token_reset_required).toBe(false)
    expect(storedConfig(store, 1).api_token_before_reset).toBeUndefined()
  })
})

describe('useConfig pending link change lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
    vi.restoreAllMocks()
  })

  it('drops the prompt when its session is deleted and recreated', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    expect(result.current.pendingLinkChange).not.toBeNull()
    await act(async () => {
      await result.current.deleteSession(1)
    })
    expect(result.current.pendingLinkChange).toBeNull()
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(result.current.config.api_base).not.toBe(ATTACKER_BASE)
    expect(storedConfig(store, 1)?.api_base).not.toBe(ATTACKER_BASE)
  })

  it('drops the prompt when every session is purged', async () => {
    window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
    const store = ownKeyStore()

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.purgeAllSessions()
    })
    expect(result.current.pendingLinkChange).toBeNull()
    await act(async () => {
      await result.current.confirmPendingLinkChange()
    })

    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
  })

  it.each(['download', 'merge'] as const)(
    'drops the prompt when a backup is imported (%s)',
    async (mode) => {
      window.history.replaceState({}, '', `/?api_base=${ATTACKER_BASE}`)
      const store = ownKeyStore()

      const { result } = await renderLoadedConfig()
      await act(async () => {
        await result.current.importAllData({}, mode)
      })
      expect(result.current.pendingLinkChange).toBeNull()
      await act(async () => {
        await result.current.confirmPendingLinkChange()
      })

      expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    },
  )
})
