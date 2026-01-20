import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessageData } from '../../types'
import { useFloatingHeader } from '../use-floating-header'

describe('useFloatingHeader', () => {
  const mockMessages: ChatMessageData[] = [
    { chatID: '1', role: 'user', content: 'hello' },
    { chatID: '2', role: 'assistant', content: 'hi' },
  ]

  const mockContainer = {
    current: {
      querySelectorAll: vi.fn(),
    },
  }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      cb(0)
      return 0
    })
  })

  it('should update message when content changes but ID is the same', () => {
    const messages = [...mockMessages]
    // Setup first message to be "active" before rendering
    const mockElement = {
      id: 'chat-message-1-user',
      getBoundingClientRect: () => ({ top: -20, bottom: 200 }),
    }
    mockContainer.current.querySelectorAll.mockReturnValue([mockElement])

    const { result, rerender } = renderHook(
      ({ msgs }) =>
        useFloatingHeader({
          messages: msgs,
          containerRef: mockContainer as any,
        }),
      { initialProps: { msgs: messages } },
    )

    act(() => {
      // Trigger update
      window.dispatchEvent(new Event('scroll'))
    })

    expect(result.current.visible).toBe(true)
    expect(result.current.chatId).toBe('1')
    expect(result.current.index).toBe(0)

    // Update message content with a NEW OBJECT reference (streaming simulation)
    const updatedMessages = [
      { ...messages[0], content: 'hello world' },
      messages[1],
    ]

    rerender({ msgs: updatedMessages })

    // We need to trigger update again or wait for the useEffect that calls updateFloatingHeader
    act(() => {
      window.dispatchEvent(new Event('scroll'))
    })

    // chatId and index should stay the same
    expect(result.current.chatId).toBe('1')
    expect(result.current.index).toBe(0)
  })

  it('should switch between messages while scrolling', () => {
    const messages = [...mockMessages]

    const element1 = {
      id: 'chat-message-1-user',
      getBoundingClientRect: () => ({ top: -100, bottom: 0 }), // scrolled out completely
    }
    const element2 = {
      id: 'chat-message-2-assistant',
      getBoundingClientRect: () => ({ top: -20, bottom: 500 }), // header out, body in
    }
    mockContainer.current.querySelectorAll.mockReturnValue([element1, element2])

    const { result } = renderHook(() =>
      useFloatingHeader({ messages, containerRef: mockContainer as any }),
    )

    act(() => {
      window.dispatchEvent(new Event('scroll'))
    })

    expect(result.current.visible).toBe(true)
    expect(result.current.chatId).toBe('2')
    expect(result.current.role).toBe('assistant')
    expect(result.current.index).toBe(1)
  })
})
