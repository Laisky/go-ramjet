import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { kvSet, SYNC_KEY } from '../../__tests__/fixtures/credential-store'
import {
  openSidebar,
  storedSession1,
} from '../../__tests__/fixtures/sidebar-harness'
import { installStore } from '../../__tests__/fixtures/storage-mock'
import { api } from '../../utils/api'
import { DataSyncManager } from '../data-sync-manager'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
  getSHA1: vi.fn(),
}))

vi.mock('../../utils/api', () => ({
  api: {
    downloadUserData: vi.fn(),
    uploadUserData: vi.fn(),
    deleteUserData: vi.fn(),
  },
}))

vi.mock('../../hooks/use-user', () => ({
  useUser: () => ({
    user: null,
    isLoading: false,
    isError: null,
    mutate: vi.fn(),
  }),
}))

vi.mock('../session-manager', () => ({
  SessionManager: () => <div data-testid="session-manager" />,
}))

vi.mock('../model-selector', () => ({
  ModelSelector: () => <div data-testid="model-selector" />,
}))

vi.mock('../prompt-shortcut-manager', () => ({
  PromptShortcutManager: () => <div data-testid="prompt-shortcuts" />,
}))

/**
 * failWritesOf makes every later write of key reject, as IndexedDB does
 * when the quota is exceeded, while other keys are still stored.
 */
function failWritesOf(store: Record<string, unknown>, key: string) {
  kvSet.mockImplementation(async (k: string, value: unknown) => {
    if (k === key) throw new Error('QuotaExceededError')
    store[k] = value
  })
}

describe('ConfigSidebar secret editors when storing fails', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })

  it('keeps the API key editor open with its draft and explains the failure', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original-key-1234' })
    failWritesOf(store, 'chat_user_config_1')

    await user.click(screen.getByRole('button', { name: 'Replace API key' }))
    const editor = screen.getByLabelText<HTMLInputElement>('API Key', {
      selector: 'input',
    })
    await user.paste('sk-typed-new-key')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      /could not save the api key/i,
    )
    expect(editor).toBeInTheDocument()
    expect(editor.value).toBe('sk-typed-new-key')
    expect(storedSession1(store).api_token).toBe('sk-original-key-1234')

    // The in-memory key reverted too: Cancel shows the stored key's hint.
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.getByText('sk-…1234')).toBeInTheDocument()
  })

  it('keeps the sync key unchanged when its config cannot be stored', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })
    failWritesOf(store, 'chat_user_config_1')

    await user.click(screen.getByRole('button', { name: 'Replace sync key' }))
    await user.paste('sync-fromotherdevice1')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      /could not save the sync key/i,
    )
    expect(store.config_sync_key).toBe(SYNC_KEY)
  })
})

describe('DataSyncManager old backup cleanup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'alert').mockImplementation(() => undefined)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })

  /**
   * renderDataSync renders DataSyncManager on its own with stubbed config
   * and cloud callbacks.
   */
  function renderDataSync() {
    render(
      <DataSyncManager
        config={{ sync_key: SYNC_KEY, api_token: 'sk-1' } as never}
        onConfigChange={vi.fn().mockResolvedValue({ rejections: [] })}
        onExportData={vi.fn().mockResolvedValue({})}
        onImportData={vi.fn().mockResolvedValue(undefined)}
      />,
    )
  }

  it('offers to delete an old backup whose delete failed, until it succeeds', async () => {
    const user = userEvent.setup()
    const oldKey = 'sync-oldoldoldoldold1'
    const store = installStore({
      config_sync_key: SYNC_KEY,
      config_pending_backup_deletions: [oldKey],
    })
    vi.mocked(api.deleteUserData).mockRejectedValueOnce(new Error('offline'))
    renderDataSync()

    const button = await screen.findByRole('button', {
      name: 'Delete old backup',
    })
    expect(screen.getByText(/sync-…old1/)).toBeInTheDocument()

    await user.click(button)
    await waitFor(() => {
      expect(window.alert).toHaveBeenCalledWith(
        expect.stringMatching(/failed/i),
      )
    })
    expect(store.config_pending_backup_deletions).toEqual([oldKey])
    expect(
      screen.getByRole('button', { name: 'Delete old backup' }),
    ).toBeInTheDocument()

    vi.mocked(api.deleteUserData).mockResolvedValueOnce(undefined)
    await user.click(screen.getByRole('button', { name: 'Delete old backup' }))

    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Delete old backup' }),
      ).not.toBeInTheDocument()
    })
    expect(api.deleteUserData).toHaveBeenLastCalledWith(oldKey)
    expect(store.config_pending_backup_deletions).toEqual([])
  })
})
