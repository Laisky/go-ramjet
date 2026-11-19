import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { LinkCredentialChangeBanner } from '../link-credential-change-alert'

/** OWN_KEY is a synthetic stored key that must never be shown in full. */
const OWN_KEY = 'sk-SYNTHETIC-OWN-KEY-0123456789'

/** LINK_KEY is a synthetic key a link carries; it is never shown in full. */
const LINK_KEY = 'sk-SYNTHETIC-LINK-KEY-9876543210'

/** config is the active session the banner describes. */
const config = {
  api_base: 'https://provider-a.example',
  api_token: OWN_KEY,
  sync_key: 'sync-k2x9q4w7p1z8m3',
}

/** alertElement returns the rendered link-change alert. */
function alertElement() {
  return screen.getByRole('alert', {
    name: 'Apply the API settings from this link?',
  })
}

describe('LinkCredentialChangeBanner', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('names both endpoints and only a masked key for an endpoint change', () => {
    render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://evil.example/v1/' }}
        config={config}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )

    const alert = alertElement()
    expect(alert).toHaveTextContent('https://evil.example/v1')
    expect(alert).toHaveTextContent('https://provider-a.example')
    expect(alert).toHaveTextContent('sk-…6789')
    expect(alert).toHaveTextContent('would be sent there too')
    expect(alert).not.toHaveTextContent(OWN_KEY)
  })

  it('names both keys masked for a key replacement', () => {
    render(
      <LinkCredentialChangeBanner
        change={{ apiToken: LINK_KEY }}
        config={config}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )

    const alert = alertElement()
    expect(alert).toHaveTextContent('replace your API key (sk-…6789)')
    expect(alert).toHaveTextContent('with sk-…3210')
    expect(alert).not.toHaveTextContent(OWN_KEY)
    expect(alert).not.toHaveTextContent(LINK_KEY)
    expect(alert).not.toHaveTextContent('would be sent there too')
  })

  it('describes the key a pending reset set aside', () => {
    render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://evil.example' }}
        config={{
          ...config,
          api_token: 'FREETIER-synthetic-0123456789',
          api_token_reset_required: true,
          api_token_before_reset: OWN_KEY,
        }}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )

    expect(alertElement()).toHaveTextContent('sk-…6789')
  })

  it('describes a change for a session without a key of the user’s own', () => {
    const freeTier = {
      ...config,
      api_token: 'FREETIER-synthetic-0123456789',
    }
    const { rerender } = render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://evil.example', apiToken: LINK_KEY }}
        config={freeTier}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )

    let alert = alertElement()
    expect(alert).toHaveTextContent('chat history')
    expect(alert).toHaveTextContent('use the API key sk-…3210')
    expect(alert).not.toHaveTextContent('()')
    expect(alert).not.toHaveTextContent('replace your API key')
    expect(alert).not.toHaveTextContent('FREETIER-')

    rerender(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://evil.example' }}
        config={freeTier}
        onConfirm={vi.fn()}
        onDismiss={vi.fn()}
      />,
    )
    alert = alertElement()
    expect(alert).toHaveTextContent('An API key you add to this session later')
    expect(alert).not.toHaveTextContent('()')
  })

  it('keeps the current settings when the user declines', async () => {
    const onConfirm = vi.fn()
    const onDismiss = vi.fn()
    render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://evil.example' }}
        config={config}
        onConfirm={onConfirm}
        onDismiss={onDismiss}
      />,
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'Keep my current settings' }),
    )
    expect(onDismiss).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('applies only on the explicit apply button', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const onDismiss = vi.fn()
    render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://provider-b.example', apiToken: LINK_KEY }}
        config={config}
        onConfirm={onConfirm}
        onDismiss={onDismiss}
      />,
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'Apply link settings' }),
    )
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(onDismiss).not.toHaveBeenCalled()
  })

  it('reports a failed change and stays usable', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const onConfirm = vi.fn().mockRejectedValue(new Error('disk full'))
    render(
      <LinkCredentialChangeBanner
        change={{ apiBase: 'https://provider-b.example' }}
        config={config}
        onConfirm={onConfirm}
        onDismiss={vi.fn()}
      />,
    )

    const button = screen.getByRole('button', { name: 'Apply link settings' })
    await userEvent.click(button)

    await waitFor(() => {
      expect(alertSpy).toHaveBeenCalledTimes(1)
    })
    expect(button).toBeEnabled()
  })
})
