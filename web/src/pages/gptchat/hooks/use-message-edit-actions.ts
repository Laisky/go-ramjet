/**
 * Edit-and-resend and insert-message dialogs of the GPTChat page.
 */
import { useCallback, useState, type RefObject } from 'react'

import type { ChatAttachment } from '../types'
import type { ScrollMode } from './use-chat-scroll'

/**
 * EditingMessage is the user message whose edit dialog is open.
 */
export interface EditingMessage {
  chatId: string
  content: string
  attachments?: ChatAttachment[]
}

/**
 * UseMessageEditActionsOptions configures useMessageEditActions:
 * editAndRetry and insertMessageAt come from useChat, canInsert tells
 * whether a message may be inserted now, and scrollModeRef is the chat
 * scroll mode of useChatScroll.
 */
export interface UseMessageEditActionsOptions {
  editAndRetry: (
    chatId: string,
    content: string,
    attachments?: ChatAttachment[],
  ) => Promise<unknown>
  insertMessageAt: (
    index: number,
    content: string,
    attachments?: ChatAttachment[],
  ) => Promise<unknown>
  canInsert: boolean
  scrollModeRef: RefObject<ScrollMode>
}

/**
 * useMessageEditActions holds the state of the edit and insert dialogs and
 * returns it with their open, confirm, and cancel handlers.
 */
export function useMessageEditActions({
  editAndRetry,
  insertMessageAt,
  canInsert,
  scrollModeRef,
}: UseMessageEditActionsOptions) {
  const [editingMessage, setEditingMessage] = useState<EditingMessage | null>(
    null,
  )
  const [insertingAt, setInsertingAt] = useState<number | null>(null)

  const handleEditResend = useCallback((payload: EditingMessage) => {
    // Opening the modal doesn't change any message content yet, so there's
    // nothing for auto-scroll to react to.  Let the current scroll mode
    // persist untouched.
    setEditingMessage({
      chatId: payload.chatId,
      content: payload.content,
      attachments: payload.attachments,
    })
  }, [])

  const handleConfirmEdit = useCallback(
    async (newContent: string, attachments?: ChatAttachment[]) => {
      if (!editingMessage) return
      setEditingMessage(null)
      // The streamingChatId anchor in useChatScroll keeps the edited message's
      // regenerated content visible when auto-follow is active, and leaves the
      // viewport alone when the user has scrolled away.  Forcing viewport-lock
      // here would instead strand the user with no auto-follow even if they
      // explicitly triggered this edit from the bottom of the conversation.
      await editAndRetry(editingMessage.chatId, newContent, attachments)
    },
    [editAndRetry, editingMessage],
  )

  const handleOpenInsert = useCallback(
    (insertAt: number) => {
      if (!canInsert) return
      setInsertingAt(insertAt)
    },
    [canInsert],
  )

  const handleConfirmInsert = useCallback(
    async (content: string, attachments?: ChatAttachment[]) => {
      if (insertingAt === null) return
      const insertIndex = insertingAt
      setInsertingAt(null)
      scrollModeRef.current = 'auto-follow'
      await insertMessageAt(insertIndex, content, attachments)
    },
    [insertMessageAt, insertingAt, scrollModeRef],
  )

  return {
    editingMessage,
    insertingAt,
    handleEditResend,
    handleConfirmEdit,
    cancelEdit: useCallback(() => setEditingMessage(null), []),
    handleOpenInsert,
    handleConfirmInsert,
    cancelInsert: useCallback(() => setInsertingAt(null), []),
  }
}
