import {
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'

import { createDeepResearchTask, fetchDeepResearchStatus } from '@/utils/api'

import { type ChatMessageData, type SessionConfig } from '../types'

export interface DeepResearchOptions {
  chatId: string
  prompt: string
  config: SessionConfig
  setMessages: Dispatch<SetStateAction<ChatMessageData[]>>
  setIsLoading: (value: boolean) => void
  setError: (value: string | null) => void
  saveMessage: (message: ChatMessageData) => Promise<void>
  deepResearchAbortRef: MutableRefObject<boolean>
  currentChatIdRef: MutableRefObject<string | null>
}

/**
 * runDeepResearch orchestrates the asynchronous deep research workflow.
 */
export async function runDeepResearch({
  chatId,
  prompt,
  config,
  setMessages,
  setIsLoading,
  setError,
  saveMessage,
  deepResearchAbortRef,
  currentChatIdRef,
}: DeepResearchOptions): Promise<void> {
  setIsLoading(true)
  deepResearchAbortRef.current = false
  currentChatIdRef.current = chatId

  const assistantMessage: ChatMessageData = {
    chatID: chatId,
    role: 'assistant',
    content: 'Researching... ⏳',
    model: config.selected_model,
    timestamp: Date.now(),
  }
  setMessages((prev) => [...prev, assistantMessage])

  const apiBase =
    config.api_base !== 'https://api.openai.com' ? config.api_base : undefined

  const updateAssistant = (text: string) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant'
          ? { ...m, content: text }
          : m,
      ),
    )
  }

  try {
    const { task_id } = await createDeepResearchTask(
      prompt.trim(),
      config.api_token,
      apiBase,
    )

    let finalText = ''
    for (let i = 0; i < 60; i += 1) {
      if (deepResearchAbortRef.current) {
        throw new Error('Deep research aborted')
      }
      const status = await fetchDeepResearchStatus(
        task_id,
        config.api_token,
        apiBase,
      )
      const statusText = (status.status || '').toLowerCase()

      if (
        ['succeeded', 'success', 'completed', 'done', 'finished'].includes(
          statusText,
        )
      ) {
        finalText =
          status.result ||
          status.output ||
          status.content ||
          status.summary ||
          JSON.stringify(status)
        break
      }

      if (['failed', 'error', 'canceled', 'cancelled'].includes(statusText)) {
        throw new Error(`Deep research failed: ${status.status}`)
      }

      updateAssistant(`Researching... (${status.status || 'pending'})`)
      await new Promise((resolve) => setTimeout(resolve, 3000))
    }

    const finalAssistant: ChatMessageData = {
      ...assistantMessage,
      content:
        finalText ||
        'Research completed but no content was returned by the service.',
      timestamp: Date.now(),
    }

    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant' ? finalAssistant : m,
      ),
    )

    await saveMessage(finalAssistant)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    setError(msg)
    setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
  } finally {
    setIsLoading(false)
    currentChatIdRef.current = null
    deepResearchAbortRef.current = false
  }
}
