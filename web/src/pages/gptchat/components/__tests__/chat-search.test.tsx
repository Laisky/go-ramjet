import { render, screen, fireEvent, act } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ChatSearch } from '../chat-search'
import type { ChatMessageData } from '../../types'
import '@testing-library/jest-dom'

// Mock TooltipWrapper to avoid Radix UI issues in tests
vi.mock('@/components/ui/tooltip-wrapper', () => ({
  TooltipWrapper: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

describe('ChatSearch', () => {
  const mockMessages: ChatMessageData[] = [
    { chatID: '1', role: 'user', content: 'apple pie', timestamp: Date.now() },
    { chatID: '2', role: 'assistant', content: 'banana bread', timestamp: Date.now() },
    { chatID: '3', role: 'user', content: 'cherry tart', timestamp: Date.now() },
  ]

  it('debounces the search query', async () => {
    vi.useFakeTimers()
    const onSelectMessage = vi.fn()

    render(<ChatSearch messages={mockMessages} onSelectMessage={onSelectMessage} />)

    // Open the search dialog
    const searchButton = screen.getByLabelText('Search messages')
    fireEvent.click(searchButton)

    const input = screen.getByPlaceholderText('Search messages...')

    // Type 'apple' rapidly
    fireEvent.change(input, { target: { value: 'apple' } })

    // Effect with debounce hasn't run yet
    expect(screen.queryByText('apple pie')).not.toBeInTheDocument()

    // Fast forward 100ms - still shouldn't have results (debounce is 200ms)
    act(() => {
      vi.advanceTimersByTime(100)
    })
    expect(screen.queryByText('apple pie')).not.toBeInTheDocument()

    // Fast forward another 150ms (total 250ms)
    act(() => {
      vi.advanceTimersByTime(150)
    })

    expect(screen.getByText('apple pie')).toBeInTheDocument()
    expect(screen.queryByText('banana bread')).not.toBeInTheDocument()

    vi.useRealTimers()
  })
})
