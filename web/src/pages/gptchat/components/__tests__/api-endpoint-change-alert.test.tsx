import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiEndpointChangeBanner } from '../api-endpoint-change-alert'

/** OWN_KEY is a synthetic stored key that must never be shown in full. */
const OWN_KEY = 'sk-SYNTHETIC-OWN-KEY-0123456789'

/** config is the active session the banner describes. */
const config = { api_base: 'https://provider-a.example', api_token: OWN_KEY }

describe('ApiEndpointChangeBanner', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('names both endpoints and only a masked key', () => {
    render(
      <ApiEndpointChangeBanner
        requestedBase="https://evil.example/v1/"
        config={config}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )

    const alert = screen.getByRole('alert', {
      name: 'Switch this session to another API endpoint?',
    })
    expect(alert).toHaveTextContent('https://evil.example/v1')
    expect(alert).toHaveTextContent('https://provider-a.example')
    expect(alert).toHaveTextContent('sk-…6789')
    expect(alert).not.toHaveTextContent(OWN_KEY)
  })

  it('keeps the current endpoint when the user declines', async () => {
    const onConfirm = vi.fn()
    const onDismiss = vi.fn()
    render(
      <ApiEndpointChangeBanner
        requestedBase="https://evil.example"
        config={config}
        onConfirm={onConfirm}
        onDismiss={onDismiss}
      />,
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'Keep current endpoint' }),
    )
    expect(onDismiss).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('switches only on the explicit switch button', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const onDismiss = vi.fn()
    render(
      <ApiEndpointChangeBanner
        requestedBase="https://provider-b.example"
        config={config}
        onConfirm={onConfirm}
        onDismiss={onDismiss}
      />,
    )

    await userEvent.click(
      screen.getByRole('button', {
        name: 'Switch to https://provider-b.example',
      }),
    )
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(onDismiss).not.toHaveBeenCalled()
  })

  it('reports a failed switch and stays usable', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const onConfirm = vi.fn().mockRejectedValue(new Error('disk full'))
    render(
      <ApiEndpointChangeBanner
        requestedBase="https://provider-b.example"
        config={config}
        onConfirm={onConfirm}
        onDismiss={vi.fn()}
      />,
    )

    const button = screen.getByRole('button', {
      name: 'Switch to https://provider-b.example',
    })
    await userEvent.click(button)

    await waitFor(() => {
      expect(alertSpy).toHaveBeenCalledTimes(1)
    })
    expect(button).toBeEnabled()
  })
})
