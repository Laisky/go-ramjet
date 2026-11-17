import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SECRET_INPUT_MASK_CLASS,
  SECRET_INPUT_OPT_OUT_ATTRIBUTES,
} from '@/components/ui/secret-input-policy'
import { SYNC_KEY } from '../../__tests__/fixtures/credential-store'
import {
  openSidebar,
  storedSession1,
} from '../../__tests__/fixtures/sidebar-harness'

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

const API_TOKEN = 'sk-original-api-key'

/**
 * simulateAutofill mimics a password manager filling value into input: it
 * writes through the native setter (bypassing React) and dispatches the
 * input and change events browsers fire. With focusFirst it also focuses
 * the field first, as extensions such as Bitwarden do; without it, focus
 * stays where it was, as with Chrome's built-in autofill.
 */
function simulateAutofill(
  input: HTMLInputElement,
  value: string,
  focusFirst = false,
) {
  if (focusFirst) input.focus()
  Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    'value',
  )?.set?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

/**
 * inputsHolding lists every input in the document whose value is secret.
 */
function inputsHolding(secret: string): HTMLInputElement[] {
  return Array.from(document.querySelectorAll('input')).filter(
    (input) => input.value === secret,
  )
}

describe('ConfigSidebar secret fields are write-only', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('never puts a stored secret into an input on open', async () => {
    await openSidebar({ api_token: API_TOKEN })

    expect(inputsHolding(API_TOKEN)).toHaveLength(0)
    expect(inputsHolding(SYNC_KEY)).toHaveLength(0)
    expect(document.querySelectorAll('input[type="password"]')).toHaveLength(0)
    expect(screen.getByText('sk-…-key')).toBeInTheDocument()
    expect(screen.getByText('sync-…z8m3')).toBeInTheDocument()
  })

  it('shows the stored key on request outside any input, then hides it', async () => {
    const user = userEvent.setup()
    await openSidebar({ api_token: API_TOKEN })

    await user.click(screen.getByRole('button', { name: 'Show API key' }))
    expect(screen.getByText(API_TOKEN).tagName).toBe('CODE')
    expect(inputsHolding(API_TOKEN)).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Hide API key' }))
    expect(screen.queryByText(API_TOKEN)).toBeNull()
  })

  it('copies the stored key without rendering it', async () => {
    const user = userEvent.setup()
    await openSidebar({ api_token: API_TOKEN })
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined)

    await user.click(screen.getByRole('button', { name: 'Copy API key' }))

    expect(writeText).toHaveBeenCalledWith(API_TOKEN)
    expect(screen.getByText('API key copied')).toHaveAttribute('role', 'status')
    expect(screen.queryByText(API_TOKEN)).toBeNull()
  })

  it('marks every editor as a masked non-credential', async () => {
    const user = userEvent.setup()
    await openSidebar({ api_token: API_TOKEN })

    await user.click(screen.getByRole('button', { name: 'Replace API key' }))
    await user.click(screen.getByRole('button', { name: 'Replace sync key' }))
    const editors = [
      screen.getByLabelText<HTMLInputElement>('API Key', { selector: 'input' }),
      screen.getByLabelText<HTMLInputElement>('Sync Key', {
        selector: 'input',
      }),
    ]

    const names = new Set<string>()
    for (const input of editors) {
      expect(input.type).toBe('text')
      expect(input.value).toBe('')
      expect(input).toHaveClass(SECRET_INPUT_MASK_CLASS)
      expect(input.name).not.toMatch(/pass|pwd/i)
      for (const [attr, value] of Object.entries(
        SECRET_INPUT_OPT_OUT_ATTRIBUTES,
      )) {
        const domAttr = attr === 'autoComplete' ? 'autocomplete' : attr
        expect(input.getAttribute(domAttr)).toBe(value)
      }
      names.add(input.name)
    }
    expect(names.size).toBe(editors.length)
  })
})

describe('ConfigSidebar secret fields vs password-manager autofill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('ignores an unfocused fill of the API key editor', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: API_TOKEN })
    await user.click(screen.getByRole('button', { name: 'Replace API key' }))
    const editor = screen.getByLabelText<HTMLInputElement>('API Key', {
      selector: 'input',
    })
    editor.blur()

    simulateAutofill(editor, 'saved-site-password')

    await waitFor(() => {
      expect(editor.value).toBe('')
    })
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(storedSession1(store).api_token).toBe(API_TOKEN)
  })

  it('still refuses the sync key when an extension focuses the editor first', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: API_TOKEN })
    await user.click(screen.getByRole('button', { name: 'Replace API key' }))
    const editor = screen.getByLabelText<HTMLInputElement>('API Key', {
      selector: 'input',
    })

    // A focused fill gets past the focus guard; nothing is stored until Save,
    // and Save checks the value against the credential policy.
    simulateAutofill(editor, SYNC_KEY, true)
    expect(storedSession1(store).api_token).toBe(API_TOKEN)

    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/sync key/i)
    expect(editor).toHaveAttribute('aria-invalid', 'true')
    expect(storedSession1(store).api_token).toBe(API_TOKEN)
  })

  it('stores a focused fill of any other value only on an explicit Save', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: API_TOKEN })
    await user.click(screen.getByRole('button', { name: 'Replace API key' }))
    const editor = screen.getByLabelText<HTMLInputElement>('API Key', {
      selector: 'input',
    })

    simulateAutofill(editor, 'saved-site-password', true)
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(storedSession1(store).api_token).toBe(API_TOKEN)
    expect(inputsHolding('saved-site-password')).toHaveLength(0)
  })
})
