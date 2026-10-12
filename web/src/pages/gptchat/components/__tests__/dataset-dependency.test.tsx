import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { StrictMode } from 'react'

import {
  GPTCHAT_SSO_STORAGE_KEY,
  GPTCHAT_SSO_STATE_KEY,
  clearOwnershipSession,
} from '../../utils/ownership-session'
import { kvGet, kvSet } from '@/utils/storage'
import { DefaultSessionConfig } from '../../types'
import { api } from '../../utils/api'
import { DatasetManager } from '../dataset-manager'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  StorageKeys: { CUSTOM_DATASET_PASSWORD: 'dataset-key' },
}))
vi.mock('../../utils/api', () => ({
  api: { listDatasets: vi.fn(), listChatbots: vi.fn(), deleteDataset: vi.fn() },
}))

describe('dataset request lifecycle', () => {
  beforeEach(() => {
    clearOwnershipSession()
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem(GPTCHAT_SSO_STORAGE_KEY, 'first-session')
    history.replaceState(null, '', '/gptchat')
    vi.mocked(kvGet).mockReset().mockResolvedValue('local-key')
    vi.mocked(kvSet).mockReset().mockResolvedValue(undefined)
    vi.mocked(api.listDatasets).mockReset().mockResolvedValue({ datasets: [] })
    vi.mocked(api.listChatbots).mockReset().mockResolvedValue({ chatbots: [] })
    vi.mocked(api.deleteDataset).mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('lists private datasets with an SSO session and no model key', async () => {
    render(
      <DatasetManager config={{ ...DefaultSessionConfig, api_token: '' }} />,
    )
    await waitFor(() => expect(api.listDatasets).toHaveBeenCalledTimes(1))
    expect(api.listDatasets).toHaveBeenCalledWith(
      'local-key',
      '',
      DefaultSessionConfig.api_base,
      'first-session',
      expect.any(AbortSignal),
    )
  })

  it('does not let a previous token response replace the current dataset list', async () => {
    let resolve!: (value: Awaited<ReturnType<typeof api.listDatasets>>) => void
    vi.mocked(api.listDatasets).mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    const config = { ...DefaultSessionConfig, api_token: 'first-token' }
    const { rerender } = render(<DatasetManager config={config} />)
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await waitFor(() => expect(api.listDatasets).toHaveBeenCalledTimes(1))
    vi.mocked(api.listDatasets).mockResolvedValueOnce({
      datasets: [{ name: 'Current dataset' }],
    })
    rerender(
      <DatasetManager config={{ ...config, api_token: 'second-token' }} />,
    )
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await screen.findByText('Current dataset')
    await act(async () => {
      resolve({ datasets: [{ name: 'Stale dataset' }] })
    })
    expect(screen.getByText('Current dataset')).toBeInTheDocument()
    expect(screen.queryByText('Stale dataset')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Refresh Datasets' }),
    ).not.toBeDisabled()
  })
  it('offers explicit sign-in and never fetches private data without a session', async () => {
    localStorage.removeItem(GPTCHAT_SSO_STORAGE_KEY)
    render(
      <DatasetManager
        config={{ ...DefaultSessionConfig, api_token: 'model-key' }}
      />,
    )
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await act(async () => {})
    expect(api.listDatasets).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Sign in with Laisky SSO' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Upload Dataset' }),
    ).toBeDisabled()
  })

  it('scrubs the callback before a private fetch and keeps the model key separate', async () => {
    history.replaceState(
      null,
      '',
      '/gptchat?tab=data&sso_token=callback-session&state=' + 'a'.repeat(64),
    )
    sessionStorage.setItem(GPTCHAT_SSO_STATE_KEY, 'a'.repeat(64))
    vi.mocked(api.listDatasets).mockImplementation(
      async (_password, key, _base, session) => {
        expect(location.search).toBe('?tab=data')
        expect(key).toBe('model-key')
        expect(session).toBe('callback-session')
        return { datasets: [] }
      },
    )
    render(
      <DatasetManager
        config={{ ...DefaultSessionConfig, api_token: 'model-key' }}
      />,
    )
    await waitFor(() => expect(api.listDatasets).toHaveBeenCalledTimes(1))
  })

  it('aborts old-account requests and removes old data immediately on a session change', async () => {
    let resolve!: (value: Awaited<ReturnType<typeof api.listDatasets>>) => void
    vi.mocked(api.listDatasets).mockResolvedValueOnce({
      datasets: [{ name: 'Old account dataset' }],
    })
    const config = { ...DefaultSessionConfig, api_token: 'same-model-key' }
    render(<DatasetManager config={config} />)
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await screen.findByText('Old account dataset')
    vi.mocked(api.listDatasets).mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Refresh Datasets' }))
    const oldSignal = vi.mocked(api.listDatasets).mock.calls[1][4]
    vi.mocked(api.listDatasets).mockResolvedValueOnce({
      datasets: [{ name: 'New account dataset' }],
    })
    act(() => {
      localStorage.setItem(GPTCHAT_SSO_STORAGE_KEY, 'second-session')
      window.dispatchEvent(
        new StorageEvent('storage', { key: GPTCHAT_SSO_STORAGE_KEY }),
      )
    })
    expect(screen.queryByText('Old account dataset')).not.toBeInTheDocument()
    expect(oldSignal?.aborted).toBe(true)
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await screen.findByText('New account dataset')
    await act(async () => {
      resolve({ datasets: [{ name: 'Stale account dataset' }] })
    })
    expect(screen.queryByText('Stale account dataset')).not.toBeInTheDocument()
    expect(screen.getByText('New account dataset')).toBeInTheDocument()
    expect(api.listDatasets).toHaveBeenLastCalledWith(
      'local-key',
      'same-model-key',
      config.api_base,
      'second-session',
      expect.any(AbortSignal),
    )
  })

  it('signing out removes private items and leaves the model configuration intact', async () => {
    const config = { ...DefaultSessionConfig, api_token: 'unchanged-model-key' }
    vi.mocked(api.listDatasets).mockResolvedValue({
      datasets: [{ name: 'Private item' }],
    })
    render(<DatasetManager config={config} />)
    fireEvent.click(screen.getByText('Private Dataset (PDF Chat)'))
    await screen.findByText('Private item')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Sign out' }))
    })
    expect(screen.queryByText('Private item')).not.toBeInTheDocument()
    expect(config.api_token).toBe('unchanged-model-key')
    expect(localStorage.getItem(GPTCHAT_SSO_STORAGE_KEY)).toBeNull()
    expect(api.listDatasets).toHaveBeenCalledTimes(1)
  })

  it('uses live request signals after StrictMode cleanup and cancels on unmount', async () => {
    vi.mocked(api.listDatasets).mockReturnValue(new Promise(() => {}))
    const { unmount } = render(
      <StrictMode>
        <DatasetManager
          config={{ ...DefaultSessionConfig, api_token: 'model-key' }}
        />
      </StrictMode>,
    )
    await waitFor(() => expect(api.listDatasets).toHaveBeenCalled())
    const signal = vi.mocked(api.listDatasets).mock.calls.at(-1)?.[4]
    expect(signal?.aborted).toBe(false)
    unmount()
    expect(signal?.aborted).toBe(true)
  })
})
