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

describe('ApiKeyResetBanner with a set-aside key that may be the user’s own', () => {
  const CUSTOM = 'my-custom-provider-token'

  beforeEach(() => {
    vi.clearAllMocks()
    installStore({})
  })

  /**
   * renderAmbiguous renders the banner for a session whose reset set CUSTOM
   * aside while it still is the sync key.
   */
  function renderAmbiguous(rotateSyncKey: ReturnType<typeof vi.fn>) {
    const updateConfig = vi.fn().mockResolvedValue({ rejections: [] })
    render(
      <ApiKeyResetBanner
        sessionId={1}
        config={sessionConfig({
          api_token: 'FREETIER-placeholder',
          api_token_reset_required: true,
          api_token_before_reset: CUSTOM,
          sync_key: CUSTOM,
        })}
        updateConfig={updateConfig}
        onOpenApiKeySettings={vi.fn()}
        rotateSyncKey={rotateSyncKey}
      />,
    )
    return updateConfig
  }

  it('rotates the sync key, after confirmation, then restores the key', async () => {
    const user = userEvent.setup()
    const order: string[] = []
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const rotateSyncKey = vi.fn(async () => {
      order.push('rotate')
      return { newKey: 'sync-n3wn3wn3wn3wn3w', oldBackupDeleted: true }
    })
    const updateConfig = renderAmbiguous(rotateSyncKey)
    updateConfig.mockImplementation(async () => {
      order.push('restore')
      return { rejections: [] }
    })

    expect(screen.getByText('Confirm this session’s API key')).toBeVisible()
    await user.click(screen.getByRole('button', { name: /It is my API key/ }))

    await waitFor(() => {
      expect(updateConfig).toHaveBeenCalledWith({ api_token: CUSTOM })
    })
    expect(rotateSyncKey).toHaveBeenCalledWith(CUSTOM)
    expect(order).toEqual(['rotate', 'restore'])
  })

  it('changes nothing when the rotation is not confirmed', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const rotateSyncKey = vi.fn()
    const updateConfig = renderAmbiguous(rotateSyncKey)

    await user.click(screen.getByRole('button', { name: /It is my API key/ }))

    expect(rotateSyncKey).not.toHaveBeenCalled()
    expect(updateConfig).not.toHaveBeenCalled()
  })

  it('reports a failed rotation and keeps the alert', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.spyOn(window, 'alert').mockImplementation(() => undefined)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const rotateSyncKey = vi.fn().mockRejectedValue(new Error('offline'))
    const updateConfig = renderAmbiguous(rotateSyncKey)

    await user.click(screen.getByRole('button', { name: /It is my API key/ }))

    await waitFor(() => {
      expect(window.alert).toHaveBeenCalledWith(
        expect.stringMatching(/restoring the api key failed/i),
      )
    })
    expect(updateConfig).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: /It is my API key/ }),
    ).toBeEnabled()
  })
})
