import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { kvGet, kvSet } from '@/utils/storage'
import { DefaultSessionConfig } from '../../types'
import { api } from '../../utils/api'
import { DatasetManager } from '../dataset-manager'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  StorageKeys: { CUSTOM_DATASET_PASSWORD: 'dataset-key' },
}))
vi.mock('../../utils/api', () => ({ api: { listDatasets: vi.fn() } }))

describe('dataset request lifecycle', () => {
  beforeEach(() => {
    vi.mocked(kvGet).mockReset().mockResolvedValue('local-key')
    vi.mocked(kvSet).mockReset().mockResolvedValue(undefined)
    vi.mocked(api.listDatasets).mockReset().mockResolvedValue({ datasets: [] })
  })
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('does not request private datasets without an API token', async () => {
    render(
      <DatasetManager config={{ ...DefaultSessionConfig, api_token: '' }} />,
    )
    await act(async () => {})
    expect(api.listDatasets).not.toHaveBeenCalled()
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
})
