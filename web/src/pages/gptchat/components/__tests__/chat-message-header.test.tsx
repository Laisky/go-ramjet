import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessageData } from '../../types'
import { ChatMessageHeader } from '../chat-message-header'

// Mock icons
vi.mock('lucide-react', () => ({
  AlertCircle: () => <div data-testid="alert-icon" />,
  Bot: () => <div data-testid="bot-icon" />,
  User: () => <div data-testid="user-icon" />,
  Check: () => <div data-testid="check-icon" />,
  Copy: () => <div data-testid="copy-icon" />,
  Edit2: () => <div data-testid="edit-icon" />,
  GitFork: () => <div data-testid="fork-icon" />,
  Loader2: () => <div data-testid="loader-icon" />,
  RotateCcw: () => <div data-testid="rotate-icon" />,
  Trash2: () => <div data-testid="trash-icon" />,
  Volume2: () => <div data-testid="volume2-icon" />,
  VolumeX: () => <div data-testid="volumex-icon" />,
  X: () => <div data-testid="x-icon" />,
}))

describe('ChatMessageHeader', () => {
  const mockMessage: ChatMessageData = {
    chatID: 'test-chat-id',
    role: 'user',
    content: 'hello world',
    timestamp: new Date('2024-01-20T10:00:00Z').getTime(),
  }

  const assistantMessage: ChatMessageData = {
    chatID: 'test-chat-id-2',
    role: 'assistant',
    content: 'i am an assistant',
  }

  beforeEach(() => {
    vi.clearAllMocks()
    // Mock navigator.clipboard
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: vi.fn().mockImplementation(() => Promise.resolve()),
      },
    })
  })

  it('renders user message correctly', () => {
    render(<ChatMessageHeader message={mockMessage} />)
    expect(screen.getByText('You')).toBeInTheDocument()
    expect(screen.getByTestId('user-icon')).toBeInTheDocument()
  })

  it('renders assistant message correctly', () => {
    render(<ChatMessageHeader message={assistantMessage} />)
    expect(screen.getByText('Assistant')).toBeInTheDocument()
    expect(screen.getByTestId('bot-icon')).toBeInTheDocument()
  })

  it('handles copy action', async () => {
    render(<ChatMessageHeader message={mockMessage} />)
    const copyButton = screen.getByTitle('Copy message')

    await act(async () => {
      fireEvent.click(copyButton)
    })

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('hello world')
    expect(screen.getByTestId('check-icon')).toBeInTheDocument()
  })

  it('requires confirmation before delete action', async () => {
    const onDelete = vi.fn()
    const user = userEvent.setup()
    render(<ChatMessageHeader message={mockMessage} onDelete={onDelete} />)
    const deleteButton = screen.getByTitle('Delete message')

    await user.click(deleteButton)
    expect(screen.getByText('Delete Message')).toBeInTheDocument()
    expect(onDelete).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith('test-chat-id')
  })

  it('handles fork action', () => {
    const onFork = vi.fn()
    render(<ChatMessageHeader message={mockMessage} onFork={onFork} />)
    const forkButton = screen.getByTitle('Fork session')

    fireEvent.click(forkButton)
    expect(onFork).toHaveBeenCalledWith('test-chat-id', 'user')
  })

  it('handles regenerate action for assistant', () => {
    const onRegenerate = vi.fn()
    render(
      <ChatMessageHeader
        message={assistantMessage}
        onRegenerate={onRegenerate}
      />,
    )
    const regenerateButton = screen.getByTitle('Regenerate response')

    fireEvent.click(regenerateButton)
    expect(onRegenerate).toHaveBeenCalledWith('test-chat-id-2')
  })

  it('handles copy failure', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: vi
          .fn()
          .mockImplementation(() => Promise.reject(new Error('failed'))),
      },
    })

    render(<ChatMessageHeader message={mockMessage} />)
    const copyButton = screen.getByRole('button', { name: /copy/i })

    await act(async () => {
      fireEvent.click(copyButton)
    })

    expect(screen.getByTestId('alert-icon')).toBeInTheDocument()
  })

  it('shows TTS error when provided', () => {
    const ttsStatus = {
      isLoading: false,
      audioUrl: null,
      error: 'API Key invalid',
      requestTTS: vi.fn(),
      stopTTS: vi.fn(),
    }
    render(
      <ChatMessageHeader
        message={assistantMessage}
        apiToken="test-token"
        ttsStatus={ttsStatus}
      />,
    )

    expect(screen.getByTestId('alert-icon')).toBeInTheDocument()
  })

  it('handles edit and resend action', () => {
    const onEditResend = vi.fn()
    render(
      <ChatMessageHeader message={mockMessage} onEditResend={onEditResend} />,
    )
    const editButton = screen.getByTitle('Edit & resend')

    fireEvent.click(editButton)
    expect(onEditResend).toHaveBeenCalledWith({
      chatId: 'test-chat-id',
      content: 'hello world',
      attachments: undefined,
    })
  })

  it('shows TTS button for assistant when API token is provided', () => {
    const ttsStatus = {
      isLoading: false,
      audioUrl: null,
      requestTTS: vi.fn(),
      stopTTS: vi.fn(),
    }
    render(
      <ChatMessageHeader
        message={assistantMessage}
        apiToken="test-token"
        ttsStatus={ttsStatus}
      />,
    )
    const ttsButton = screen.getByTitle('Play narration')
    expect(ttsButton).toBeInTheDocument()

    fireEvent.click(ttsButton)
    expect(ttsStatus.requestTTS).toHaveBeenCalledWith('i am an assistant')
  })

  it('handles TTS stop when audio is playing', () => {
    const ttsStatus = {
      isLoading: false,
      audioUrl: 'blob:xxx',
      requestTTS: vi.fn(),
      stopTTS: vi.fn(),
    }
    render(
      <ChatMessageHeader
        message={assistantMessage}
        apiToken="test-token"
        ttsStatus={ttsStatus}
      />,
    )
    const stopButton = screen.getByTitle('Stop narration')
    expect(screen.getByTestId('volumex-icon')).toBeInTheDocument()

    fireEvent.click(stopButton)
    expect(ttsStatus.stopTTS).toHaveBeenCalled()
  })

  it('disables actions when streaming', () => {
    const onRegenerate = vi.fn()
    render(
      <ChatMessageHeader
        message={assistantMessage}
        onRegenerate={onRegenerate}
        isStreaming={true}
      />,
    )
    const regenerateButton = screen.getByTitle('Regenerate response')
    expect(regenerateButton).toBeDisabled()
  })
})
