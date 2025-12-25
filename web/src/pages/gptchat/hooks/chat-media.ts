import {
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'

import { editImageWithMask } from '@/utils/api'

import { type ChatMessageData, type SessionConfig } from '../types'

export interface MaskPair {
  image: File
  mask: File
}

export interface MaskInpaintingOptions {
  chatId: string
  prompt: string
  maskPair: MaskPair
  config: SessionConfig
  fileToDataUrl: (file: File) => Promise<string>
  setMessages: Dispatch<SetStateAction<ChatMessageData[]>>
  setIsLoading: (value: boolean) => void
  setError: (value: string | null) => void
  saveMessage: (message: ChatMessageData) => Promise<void>
  currentChatIdRef: MutableRefObject<string | null>
}

export interface ImageModelOptions {
  chatId: string
  prompt: string
  config: SessionConfig
  setMessages: Dispatch<SetStateAction<ChatMessageData[]>>
  setIsLoading: (value: boolean) => void
  setError: (value: string | null) => void
  saveMessage: (message: ChatMessageData) => Promise<void>
  currentChatIdRef: MutableRefObject<string | null>
}

/**
 * runMaskInpainting handles image editing when both image and mask are provided.
 */
export async function runMaskInpainting({
  chatId,
  prompt,
  maskPair,
  config,
  fileToDataUrl,
  setMessages,
  setIsLoading,
  setError,
  saveMessage,
  currentChatIdRef,
}: MaskInpaintingOptions): Promise<void> {
  setIsLoading(true)
  currentChatIdRef.current = chatId

  const assistantMessage: ChatMessageData = {
    chatID: chatId,
    role: 'assistant',
    content: 'Editing image...',
    model: config.selected_model,
    timestamp: Date.now(),
  }
  setMessages((prev) => [...prev, assistantMessage])

  try {
    const [imageDataUrl, maskDataUrl] = await Promise.all([
      fileToDataUrl(maskPair.image),
      fileToDataUrl(maskPair.mask),
    ])

    const resp = await editImageWithMask(
      'flux-fill-pro',
      {
        prompt: prompt.trim(),
        image: imageDataUrl,
        mask: maskDataUrl,
      },
      config.api_token,
      config.api_base !== 'https://api.openai.com'
        ? config.api_base
        : undefined,
    )

    const imgContent = resp.image_urls
      .map((url) => `![Image](${url})`)
      .join('\n\n')

    const finalAssist: ChatMessageData = {
      ...assistantMessage,
      content: imgContent,
    }

    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant' ? finalAssist : m,
      ),
    )

    await saveMessage(finalAssist)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    setError(msg)
    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant'
          ? { ...m, error: msg }
          : m,
      ),
    )
  } finally {
    setIsLoading(false)
    currentChatIdRef.current = null
  }
}

/**
 * runImageModelFlow generates images for image-capable models.
 */
export async function runImageModelFlow({
  chatId,
  prompt,
  config,
  setMessages,
  setIsLoading,
  setError,
  saveMessage,
  currentChatIdRef,
}: ImageModelOptions): Promise<void> {
  setIsLoading(true)
  currentChatIdRef.current = chatId

  const assistantMessage: ChatMessageData = {
    chatID: chatId,
    role: 'assistant',
    content: 'Generating image...',
    model: config.selected_model,
    timestamp: Date.now(),
  }
  setMessages((prev) => [...prev, assistantMessage])

  try {
    const { generateImage } = await import('@/utils/api')
    const resp = await generateImage(
      {
        model: config.selected_model,
        prompt,
        n: config.chat_switch.draw_n_images,
        size: '1024x1024',
      },
      config.api_token,
      config.api_base !== 'https://api.openai.com'
        ? config.api_base
        : undefined,
    )

    const newContent = resp.data
      .map((d) =>
        d.url
          ? `![Image](${d.url})`
          : `![Image](data:image/png;base64,${d.b64_json})`,
      )
      .join('\n\n')

    const finalAssistant: ChatMessageData = {
      ...assistantMessage,
      content: newContent,
    }

    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant' ? finalAssistant : m,
      ),
    )

    await saveMessage(finalAssistant)
  } catch (err: unknown) {
    const errMsg = err instanceof Error ? err.message : String(err)
    setError(errMsg)
    setMessages((prev) =>
      prev.map((m) =>
        m.chatID === chatId && m.role === 'assistant'
          ? { ...m, error: errMsg }
          : m,
      ),
    )
  } finally {
    setIsLoading(false)
    currentChatIdRef.current = null
  }
}
