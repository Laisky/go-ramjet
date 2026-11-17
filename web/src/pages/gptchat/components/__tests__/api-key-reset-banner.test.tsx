import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  PROVIDER_A,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { useConfig } from '../../hooks/use-config'
import { ApiKeyResetBanner } from '../api-key-reset-alert'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

// The banner appears after two chained storage reads (config load, then the
// sibling scan), which can exceed the default 1 s under full-suite load.
const STORAGE_FLOW_TIMEOUT_MS = 4000

/**
 * BannerHarness renders the banner for the real useConfig state, the way the
 * GPTChat page does.
 */
function BannerHarness({ onOpen }: { onOpen: () => void }) {
  const { config, sessionId, updateConfig, isLoading } = useConfig()
  if (isLoading) return null
  return (
    <>
      <output data-testid="api-token-kind">
        {config.api_token.startsWith('FREETIER-')
          ? 'freetier'
          : config.api_token}
      </output>
      <ApiKeyResetBanner
        sessionId={sessionId}
        config={config}
        updateConfig={updateConfig}
        onOpenApiKeySettings={onOpen}
      />
    </>
  )
}

/**
 * seedClobberedSession stores session 1 with a clobbered token and session 2
 * with a key on the same endpoint. It returns the live store.
 */
function seedClobberedSession(): Record<string, unknown> {
  return installStore({
    config_selected_session: 1,
    config_sync_key: SYNC_KEY,
    chat_user_config_1: sessionConfig({
      api_base: PROVIDER_A,
      api_token: SYNC_KEY,
    }),
    chat_user_config_2: sessionConfig({
      api_base: PROVIDER_A,
      api_token: 'sk-account-b-key',
      session_name: 'Work',
    }),
  })
}

describe('ApiKeyResetBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('adopts a sibling key only when the user picks it, in one write', async () => {
    const user = userEvent.setup()
    const store = seedClobberedSession()
    render(<BannerHarness onOpen={vi.fn()} />)

    const choice = await screen.findByRole(
      'button',
      { name: 'Use sk-…-key from Work' },
      { timeout: STORAGE_FLOW_TIMEOUT_MS },
    )
    expect(screen.getByTestId('api-token-kind')).toHaveTextContent('freetier')
    expect(storedConfig(store, 1).api_token).toMatch(/^FREETIER-/)

    await user.click(choice)

    await waitFor(() => {
      expect(screen.queryByRole('alert')).toBeNull()
    })
    expect(storedConfig(store, 1)).toMatchObject({
      api_token: 'sk-account-b-key',
      api_token_reset_required: false,
    })
  })

  it('drops a choice that is no longer valid instead of applying it', async () => {
    const user = userEvent.setup()
    const store = seedClobberedSession()
    render(<BannerHarness onOpen={vi.fn()} />)
    const choice = await screen.findByRole(
      'button',
      { name: 'Use sk-…-key from Work' },
      { timeout: STORAGE_FLOW_TIMEOUT_MS },
    )

    delete store.chat_user_config_2
    await user.click(choice)

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^Use / })).toBeNull()
    })
    expect(storedConfig(store, 1).api_token).toMatch(/^FREETIER-/)
    expect(storedConfig(store, 1).api_token_reset_required).toBe(true)
  })

  it('opens the API key editor from the alert', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn()
    seedClobberedSession()
    render(<BannerHarness onOpen={onOpen} />)

    await user.click(
      await screen.findByRole(
        'button',
        { name: 'Re-enter API key' },
        { timeout: STORAGE_FLOW_TIMEOUT_MS },
      ),
    )
    expect(onOpen).toHaveBeenCalledTimes(1)
  })
})
