import { Button } from '@/components/ui/button'
import React, { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatAttachment } from '../types'
import { MessageInput } from './message-input'

interface EditMessageModalProps {
  content: string
  attachments?: ChatAttachment[]
  onClose: () => void
  onConfirm: (newContent: string, attachments?: ChatAttachment[]) => void
  apiToken: string
}

/**
 * EditMessageModal allows users to edit a message and its attachments before resending.
 */
export function EditMessageModal({
  content,
  attachments,
  onClose,
  onConfirm,
  apiToken,
}: EditMessageModalProps) {
  const [editedContent, setEditedContent] = useState(content)
  const [editedAttachments, setEditedAttachments] = useState(attachments || [])
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
    textareaRef.current?.select()
  }, [])

  const handleSubmit = useCallback(() => {
    const trimmed = String(editedContent || '').trim()
    if (trimmed) {
      onConfirm(trimmed, editedAttachments)
    }
  }, [editedContent, editedAttachments, onConfirm])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // Ignore keyboard events when composition is in progress (IME)
      if (e.nativeEvent.isComposing) return

      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSubmit()
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    },
    [handleSubmit, onClose],
  )

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="mx-4 w-full max-w-2xl rounded-lg border theme-border theme-elevated p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-4 text-lg font-semibold">Edit Message</h3>

        <MessageInput
          textareaRef={textareaRef}
          value={editedContent}
          onChange={setEditedContent}
          attachments={editedAttachments}
          onAttachmentsChange={setEditedAttachments}
          apiToken={apiToken}
          onKeyDown={handleKeyDown}
          className="mb-4"
          rows={10}
        />

        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!String(editedContent || '').trim()}
          >
            Retry with Edited Message
          </Button>
        </div>
        <p className="mt-2 text-xs theme-text-muted">
          Ctrl+Enter to submit • Esc to cancel
        </p>
      </div>
    </div>
  )
}
