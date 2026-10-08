import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GPTChatPage } from '..'
import type { ChatMessageData } from '../types'
import { DefaultSessionConfig } from '../types'

const state = vi.hoisted(() => ({
  messages: [] as ChatMessageData[],
  isLoading: false,
  loadingChatId: null as string | null,
  apiToken: 'test-api-key',
  visibleCount: 40,
  insertMessageAt: vi.fn(),
  loadMessages: vi.fn(),
  scrollToBottom: vi.fn(),
  scrollModeRef: { current: 'auto-follow' },
}))

// Exercise the real page and insertion control without network, storage, or
// unrelated chat chrome. Message updates model the useChat streaming lifecycle.
vi.mock('../hooks/use-chat', () => ({ useChat: () => state }))
vi.mock('../hooks/use-config', () => ({
  useConfig: () => ({
    config: { ...DefaultSessionConfig, api_token: state.apiToken },
    sessionId: 1,
    sessions: [],
    isLoading: false,
  }),
}))
vi.mock('../hooks/use-chat-scroll', () => ({
  useChatScroll: () => ({
    visibleCount: state.visibleCount,
    messagesEndRef: { current: null },
    scrollModeRef: state.scrollModeRef,
    scrollToBottom: state.scrollToBottom,
  }),
}))
vi.mock('../hooks/use-draft', () => ({ useDraft: () => ({}) }))
vi.mock('../hooks/use-floating-header', () => ({
  useFloatingHeader: () => ({}),
}))
vi.mock('../hooks/use-mcp-sync', () => ({ useMcpSync: vi.fn() }))
vi.mock('../hooks/use-message-navigation', () => ({
  useMessageNavigation: () => ({}),
}))
vi.mock('../hooks/use-prompt-shortcuts', () => ({
  usePromptShortcuts: () => ({}),
}))
vi.mock('../hooks/use-selection', () => ({ useSelection: () => ({}) }))
vi.mock('../hooks/use-tts', () => ({ useTTS: () => ({}) }))
vi.mock('../hooks/use-user', () => ({ useUser: () => ({}) }))
vi.mock('../hooks/use-version-check', () => ({
  useVersionCheck: () => ({}),
}))
vi.mock('../utils/session-export', () => ({ exportSessionToXml: vi.fn() }))
vi.mock('@/components/theme-toggle', () => ({ ThemeToggle: () => null }))
vi.mock('@/components/ui/tooltip-wrapper', () => ({
  TooltipWrapper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('../components', async () => {
  const { ConversationInsert } = await import(
    '../components/conversation-insert'
  )
  return {
    ConversationInsert,
    ChatMessage: ({
      message,
      isStreaming,
    }: {
      message: ChatMessageData
      isStreaming: boolean
    }) => (
      <article
        aria-label={`${message.chatID}-${message.role}`}
        data-streaming={isStreaming}
      >
        {message.content}
      </article>
    ),
    ChatInput: ({
      onCallSessionChange,
    }: {
      onCallSessionChange: (sessionId: number) => void
    }) => (
      <button onClick={() => onCallSessionChange(1)}>Start voice call</button>
    ),
    ChatSearch: () => null,
    FloatingMessageHeader: () => null,
    ModelSelector: () => null,
    SelectionTTSPlayer: () => null,
    SelectionToolbar: () => null,
    SessionDock: () => null,
    UpgradeNotification: () => null,
  }
})

// Keep the page's open/confirm handlers real; only replace the editor's UI.
vi.mock('../components/edit-message-modal', () => ({
  EditMessageModal: ({
    onConfirm,
    submitLabel,
  }: {
    onConfirm: (content: string) => Promise<void>
    submitLabel: string
  }) => (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        const data = new FormData(event.currentTarget)
        void onConfirm(String(data.get('message')))
      }}
    >
      <input aria-label="Inserted request" name="message" />
      <button type="submit">{submitLabel}</button>
    </form>
  ),
}))

/** makeMessage returns a message fixture for the supplied ID, role, and text. */
function makeMessage(
  chatID: string,
  role: ChatMessageData['role'],
  content = `${role} ${chatID}`,
): ChatMessageData {
  return { chatID, role, content }
}

/** expectInsertTargets asserts the message labels immediately after controls. */
function expectInsertTargets(labels: string[]) {
  const buttons = screen.queryAllByRole('button', {
    name: 'Insert message here',
  })
  expect(
    buttons.map((button) =>
      button.parentElement?.nextElementSibling?.getAttribute('aria-label'),
    ),
  ).toEqual(labels)
}

/** insertBefore submits content through the control before the given user ID. */
async function insertBefore(chatID: string, content: string) {
  const user = userEvent.setup()
  const message = screen.getByRole('article', { name: `${chatID}-user` })
  const boundary = message.previousElementSibling
  expect(boundary).toBeInstanceOf(HTMLElement)
  await user.click(
    within(boundary as HTMLElement).getByRole('button', {
      name: 'Insert message here',
    }),
  )
  await user.type(
    await screen.findByRole('textbox', { name: 'Inserted request' }),
    content,
  )
  await user.click(screen.getByRole('button', { name: 'Send from here' }))
}

describe('GPTChatPage conversation insertion boundaries', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.messages = [
      makeMessage('first', 'user'),
      makeMessage('first', 'assistant'),
      makeMessage('second', 'user'),
      makeMessage('second', 'assistant'),
    ]
    state.isLoading = false
    state.loadingChatId = null
    state.apiToken = 'test-api-key'
    state.visibleCount = 40
    state.scrollModeRef.current = 'auto-follow'
    state.insertMessageAt.mockResolvedValue(undefined)
  })

  afterEach(cleanup)

  it('offers insertion before user requests, never inside request/response pairs', () => {
    render(<GPTChatPage />)
    expectInsertTargets(['second-user'])
  })

  it('does not add an insertion control to a single request/response pair', () => {
    state.messages = state.messages.slice(0, 2)
    render(<GPTChatPage />)
    expectInsertTargets([])
  })

  it('filters by role rather than assuming alternating user and assistant messages', () => {
    state.messages = [
      makeMessage('first', 'user'),
      makeMessage('system', 'system'),
      makeMessage('first', 'assistant'),
      makeMessage('extra', 'assistant'),
      makeMessage('second', 'user'),
      makeMessage('third', 'user'),
    ]
    render(<GPTChatPage />)
    expectInsertTargets(['second-user', 'third-user'])
  })

  it('keeps inserted requests adjacent to empty, streaming, and completed replies', async () => {
    const { rerender, unmount } = render(<GPTChatPage />)
    await insertBefore('second', 'A follow-up question')
    expect(state.insertMessageAt).toHaveBeenCalledExactlyOnceWith(
      2,
      'A follow-up question',
      undefined,
    )

    const original = state.messages
    for (const content of ['', 'Partial answer', 'Complete answer']) {
      state.isLoading = content !== 'Complete answer'
      state.loadingChatId = state.isLoading ? 'inserted' : null
      state.messages = [
        ...original.slice(0, 2),
        makeMessage('inserted', 'user', 'A follow-up question'),
        makeMessage('inserted', 'assistant', content),
        ...original.slice(2),
      ]
      rerender(<GPTChatPage />)

      expectInsertTargets(['inserted-user', 'second-user'])
      const response = screen.getByRole('article', {
        name: 'inserted-assistant',
      })
      expect(response.previousElementSibling).toBe(
        screen.getByRole('article', { name: 'inserted-user' }),
      )
      expect(response).toHaveAttribute(
        'data-streaming',
        String(state.isLoading),
      )
      for (const button of screen.getAllByRole('button', {
        name: 'Insert message here',
      })) {
        if (state.isLoading) {
          expect(button).toBeDisabled()
        } else {
          expect(button).toBeEnabled()
        }
      }
    }

    unmount()
    render(<GPTChatPage />)
    expectInsertTargets(['inserted-user', 'second-user'])
  })

  it('preserves the absolute insertion index when older messages are hidden', async () => {
    state.visibleCount = 3
    render(<GPTChatPage />)
    expectInsertTargets(['second-user'])
    await insertBefore('second', 'Insert into paged history')
    expect(state.insertMessageAt).toHaveBeenCalledExactlyOnceWith(
      2,
      'Insert into paged history',
      undefined,
    )
  })

  it('preserves the existing first-visible-message boundary behavior', () => {
    state.visibleCount = 2
    render(<GPTChatPage />)
    expectInsertTargets([])
  })

  it('keeps valid insertion controls disabled without an API key', async () => {
    state.apiToken = ''
    render(<GPTChatPage />)
    expectInsertTargets(['second-user'])
    const button = screen.getByRole('button', { name: 'Insert message here' })
    expect(button).toBeDisabled()
    await userEvent.setup().click(button)
    expect(state.insertMessageAt).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('keeps valid insertion controls disabled during a voice call', async () => {
    render(<GPTChatPage />)
    await userEvent.setup().click(
      screen.getByRole('button', { name: 'Start voice call' }),
    )
    expectInsertTargets(['second-user'])
    expect(
      screen.getByRole('button', { name: 'Insert message here' }),
    ).toBeDisabled()
  })
})
