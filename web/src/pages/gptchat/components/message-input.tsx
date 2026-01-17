import { Textarea } from '@/components/ui/textarea'
import { uploadFile } from '@/utils/api'
import { cn } from '@/utils/cn'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatAttachment } from '../types'
import { fileToDataUrl } from '../utils/format'
import { AttachmentTag } from './attachment-tag'
import { ImageEditorModal, type ImageEditorResult } from './image-editor-modal'

export interface MessageInputProps {
  value: string
  onChange: (value: string) => void
  attachments: ChatAttachment[]
  onAttachmentsChange: (attachments: ChatAttachment[]) => void
  placeholder?: string
  disabled?: boolean
  apiToken: string
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onKeyUp?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onMouseUp?: (e: React.MouseEvent<HTMLTextAreaElement>) => void
  onSelect?: (e: React.SyntheticEvent<HTMLTextAreaElement>) => void
  onBlur?: (e: React.FocusEvent<HTMLTextAreaElement>) => void
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>
  autoFocus?: boolean
  className?: string
  rows?: number
}

const SUPPORTED_DOC_EXTS = new Set([
  '.txt',
  '.md',
  '.doc',
  '.docx',
  '.pdf',
  '.ppt',
  '.pptx',
])
const SUPPORTED_IMAGE_EXTS = new Set([
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.webp',
  '.bmp',
  '.tif',
  '.tiff',
])

export function MessageInput({
  value,
  onChange,
  attachments,
  onAttachmentsChange,
  placeholder = 'Type a message...',
  disabled,
  apiToken,
  onKeyDown,
  onKeyUp,
  onMouseUp,
  onSelect,
  onBlur,
  textareaRef: externalTextareaRef,
  autoFocus,
  className,
  rows = 3,
}: MessageInputProps) {
  const internalTextareaRef = useRef<HTMLTextAreaElement>(null)
  const textareaRef = externalTextareaRef || internalTextareaRef
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [editorIndex, setEditorIndex] = useState<number | null>(null)
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const [editorFile, setEditorFile] = useState<File | null>(null)
  const [isUploading, setIsUploading] = useState(false)

  // Auto-resize textarea
  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    const maxHeight = 400
    textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`
  }, [value, textareaRef])

  useEffect(() => {
    if (autoFocus && !disabled) {
      textareaRef.current?.focus()
    }
  }, [autoFocus, disabled, textareaRef])

  const handleProcessFiles = useCallback(
    async (files: File[]) => {
      if (!files.length) return

      const newAttachments = [...attachments]
      let newContent = value

      for (const file of files) {
        const ext = file.name.slice(file.name.lastIndexOf('.')).toLowerCase()

        if (SUPPORTED_IMAGE_EXTS.has(ext) || file.type.startsWith('image/')) {
          try {
            const b64 = await fileToDataUrl(file)
            newAttachments.push({
              filename: file.name,
              type: 'image',
              contentB64: b64,
              file: file,
            })
          } catch (err) {
            console.error('Failed to process image:', err)
            alert(`Failed to process image ${file.name}`)
          }
        } else if (SUPPORTED_DOC_EXTS.has(ext)) {
          setIsUploading(true)
          try {
            const { url } = await uploadFile(file, apiToken)
            newAttachments.push({
              filename: file.name,
              type: 'file',
              url: url,
              file: file,
            })
            // Following chat.js requirement: automatically generate a file URL and append it to input
            newContent = url + '\n' + (newContent || '')
          } catch (err) {
            console.error('Failed to upload file:', err)
            alert(`Failed to upload file ${file.name}`)
          } finally {
            setIsUploading(false)
          }
        } else {
          alert(`Unsupported file type: ${file.name}`)
        }
      }

      onAttachmentsChange(newAttachments)
      if (newContent !== value) {
        onChange(newContent)
      }
    },
    [attachments, onAttachmentsChange, value, onChange, apiToken],
  )

  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const items = Array.from(e.clipboardData?.items || [])
      const files: File[] = []
      items.forEach((item) => {
        if (item.kind === 'file') {
          const file = item.getAsFile()
          if (file) files.push(file)
        }
      })
      if (files.length > 0) {
        void handleProcessFiles(files)
      }
    },
    [handleProcessFiles],
  )

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      if (e.dataTransfer?.files?.length) {
        void handleProcessFiles(Array.from(e.dataTransfer.files))
      }
    },
    [handleProcessFiles],
  )

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
  }, [])

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (files) {
        void handleProcessFiles(Array.from(files))
      }
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
    },
    [handleProcessFiles],
  )

  const removeAttachment = useCallback(
    (index: number) => {
      onAttachmentsChange(attachments.filter((_, i) => i !== index))
    },
    [attachments, onAttachmentsChange],
  )

  const openEditor = useCallback(
    async (index: number) => {
      const att = attachments[index]
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
    [attachments],
  )

  const handleEditorSave = useCallback(
    async (result: ImageEditorResult) => {
      if (editorIndex === null) return

      try {
        const b64 = await fileToDataUrl(result.imageFile)
        const next = [...attachments]
        next[editorIndex] = {
          ...next[editorIndex],
          contentB64: b64,
          url: undefined, // Clear URL if we have new B64 content
        }

        // If there's a mask, add it as a new attachment
        if (result.maskFile) {
          const maskB64 = await fileToDataUrl(result.maskFile)
          next.splice(editorIndex + 1, 0, {
            filename: result.maskFile.name,
            type: 'image',
            contentB64: maskB64,
          })
        }

        onAttachmentsChange(next)
        setIsEditorOpen(false)
        setEditorIndex(null)
        setEditorFile(null)
      } catch (err) {
        console.error('Failed to save edited image:', err)
      }
    },
    [attachments, editorIndex, onAttachmentsChange],
  )

  return (
    <div
      className={cn('flex flex-col gap-2', className)}
      onDrop={handleDrop}
      onDragOver={handleDragOver}
    >
      {attachments.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {attachments.map((att, i) => (
            <AttachmentTag
              key={`${att.filename}-${i}`}
              filename={att.filename}
              type={att.type}
              contentB64={att.contentB64}
              url={att.url}
              onRemove={() => removeAttachment(i)}
              onEdit={att.type === 'image' ? () => openEditor(i) : undefined}
            />
          ))}
          {isUploading && (
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-2 py-1 text-xs text-muted-foreground">
              Uploading...
            </div>
          )}
        </div>
      )}

      <div className="relative group">
        <Textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          onKeyUp={onKeyUp}
          onMouseUp={onMouseUp}
          onSelect={onSelect}
          onBlur={onBlur}
          onPaste={handlePaste}
          placeholder={placeholder}
          disabled={disabled || isUploading}
          className="min-h-[80px] w-full resize-none rounded-md border bg-background px-3 py-2 text-base focus-visible:ring-1"
          rows={rows}
        />
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*,.pdf,.doc,.docx,.txt,.md"
          onChange={handleFileSelect}
          className="hidden"
        />
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={disabled || isUploading}
          className="absolute right-2 bottom-2 text-muted-foreground hover:text-foreground disabled:opacity-50 transition-colors"
          title="Attach files (Images, PDF, Doc, Text)"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.51a2 2 0 0 1-2.83-2.83l8.49-8.48" />
          </svg>
        </button>
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
