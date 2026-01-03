import { Button } from '@/components/ui/button'
import React, { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatAttachment } from '../types'
import { AttachmentTag } from './attachment-tag'
import { ImageEditorModal } from './image-editor-modal'

interface EditMessageModalProps {
  content: string
  attachments?: ChatAttachment[]
  onClose: () => void
  onConfirm: (newContent: string, attachments?: ChatAttachment[]) => void
}

/**
 * EditMessageModal allows users to edit a message and its attachments before resending.
 */
export function EditMessageModal({
  content,
  attachments,
  onClose,
  onConfirm,
}: EditMessageModalProps) {
  const [editedContent, setEditedContent] = useState(content)
  const [editedAttachments, setEditedAttachments] = useState(attachments || [])
  const [editorIndex, setEditorIndex] = useState<number | null>(null)
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const [editorFile, setEditorFile] = useState<File | null>(null)
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

  const handleRemoveAttachment = useCallback((index: number) => {
    setEditedAttachments((prev) => prev.filter((_, i) => i !== index))
  }, [])

  const handleEditAttachment = useCallback(
    async (index: number) => {
      const att = editedAttachments[index]
      if (att.type !== 'image') return

      try {
        let file: File | null = null
        if (att.contentB64) {
          const res = await fetch(att.contentB64)
          const blob = await res.blob()
          file = new File([blob], att.filename, { type: 'image/png' })
        } else if (att.url) {
          const res = await fetch(att.url)
          const blob = await res.blob()
          file = new File([blob], att.filename, { type: 'image/png' })
        }

        if (file) {
          setEditorFile(file)
          setEditorIndex(index)
          setIsEditorOpen(true)
        }
      } catch (err) {
        console.error('Failed to prepare image for editing:', err)
      }
    },
    [editedAttachments],
  )

  const handleEditorSave = useCallback(
    async (result: { imageFile: File }) => {
      if (editorIndex === null) return

      const reader = new FileReader()
      reader.onloadend = () => {
        const base64 = reader.result as string
        setEditedAttachments((prev) => {
          const next = [...prev]
          next[editorIndex] = {
            ...next[editorIndex],
            contentB64: base64,
            url: undefined, // Clear URL if we have new B64 content
          }
          return next
        })
        setIsEditorOpen(false)
        setEditorIndex(null)
        setEditorFile(null)
      }
      reader.readAsDataURL(result.imageFile)
    },
    [editorIndex],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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

        {editedAttachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {editedAttachments.map((att, i) => (
              <AttachmentTag
                key={i}
                filename={att.filename}
                type={att.type}
                contentB64={att.contentB64}
                url={att.url}
                onRemove={() => handleRemoveAttachment(i)}
                onEdit={
                  att.type === 'image'
                    ? () => handleEditAttachment(i)
                    : undefined
                }
              />
            ))}
          </div>
        )}

        <textarea
          ref={textareaRef}
          value={editedContent}
          onChange={(e) => setEditedContent(e.target.value)}
          onKeyDown={handleKeyDown}
          className="theme-input theme-focus-ring w-full rounded border p-3 font-mono text-sm focus:outline-none focus:ring-2"
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

      <ImageEditorModal
        open={isEditorOpen}
        file={editorFile}
        onClose={() => {
          setIsEditorOpen(false)
          setEditorIndex(null)
          setEditorFile(null)
        }}
        onSave={handleEditorSave}
      />
    </div>
  )
}
