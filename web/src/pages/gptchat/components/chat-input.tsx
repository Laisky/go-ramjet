/**
 * Chat input component with file attachments and feature toggles.
 */
import {
  Edit2,
  Image,
  Link,
  Loader2,
  Mic,
  Paperclip,
  Send,
  Square,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { transcribeAudio } from '@/utils/api'
import { cn } from '@/utils/cn'
import type { SessionConfig } from '../types'
import { ImageEditorModal, type ImageEditorResult } from './image-editor-modal'

function formatFileSize(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

interface FileItemProps {
  file: File
  index: number
  onRemove: (index: number) => void
  onEdit?: (index: number) => void
}

function FileItem({ file, index, onRemove, onEdit }: FileItemProps) {
  const [url, setUrl] = useState<string | null>(null)
  const isImage = file.type.startsWith('image/')

  useEffect(() => {
    if (isImage) {
      const objectUrl = URL.createObjectURL(file)
      setUrl(objectUrl)
      return () => URL.revokeObjectURL(objectUrl)
    }
  }, [file, isImage])

  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-2 py-1 text-xs shadow-sm">
      {isImage && url ? (
        <div className="h-8 w-8 shrink-0 overflow-hidden rounded border border-border bg-background">
          <img
            src={url}
            alt={file.name}
            className="h-full w-full object-cover"
          />
        </div>
      ) : (
        <Paperclip className="h-3 w-3" />
      )}
      <div className="max-w-[180px] truncate">
        <div className="truncate font-medium">{file.name}</div>
        <div className="text-[10px] text-muted-foreground">
          {formatFileSize(file.size)}
        </div>
      </div>
      {isImage && onEdit && (
        <button
          type="button"
          onClick={() => onEdit(index)}
          className="flex items-center gap-1 rounded-md bg-popover px-1.5 py-0.5 text-[10px] text-popover-foreground shadow-sm"
        >
          <Edit2 className="h-3 w-3" />
          Edit
        </button>
      )}
      <button
        type="button"
        onClick={() => onRemove(index)}
        className="text-destructive transition hover:text-destructive/80"
      >
        ×
      </button>
    </div>
  )
}

export interface ChatInputProps {
  onSend: (message: string, files?: File[]) => void
  onStop?: () => void
  isLoading?: boolean
  disabled?: boolean
  config: SessionConfig
  sessionId?: string | number
  isSidebarOpen?: boolean
  onConfigChange?: (updates: Partial<SessionConfig['chat_switch']>) => void
  placeholder?: string
  prefillDraft?: { id: string; text: string }
  onPrefillUsed?: (id: string) => void
  draftMessage?: string
  onDraftChange?: (value: string) => void
}

/**
 * ChatInput provides the message input area with feature toggles.
 */
export function ChatInput({
  onSend,
  onStop,
  isLoading,
  disabled,
  config,
  sessionId,
  isSidebarOpen,
  onConfigChange,
  placeholder = 'Type a message...',
  prefillDraft,
  onPrefillUsed,
  draftMessage,
  onDraftChange,
}: ChatInputProps) {
  const [message, setMessage] = useState(() => draftMessage ?? '')
  const [attachedFiles, setAttachedFiles] = useState<File[]>([])
  const [isRecording, setIsRecording] = useState(false)
  const [isTranscribing, setIsTranscribing] = useState(false)
  const [editorIndex, setEditorIndex] = useState<number | null>(null)
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const recordedChunksRef = useRef<Blob[]>([])
  const lastPrefillIdRef = useRef<string | null>(null)

  const appendFiles = useCallback((files: File[]) => {
    if (!files.length) return
    setAttachedFiles((prev) => [...prev, ...files])
  }, [])

  const closeEditor = useCallback(() => {
    setIsEditorOpen(false)
    setEditorIndex(null)
  }, [])

  const openEditorForIndex = useCallback((index: number) => {
    setEditorIndex(index)
    setIsEditorOpen(true)
  }, [])

  const handleEditorSave = useCallback(
    (result: ImageEditorResult) => {
      setAttachedFiles((prev) => {
        if (editorIndex === null) return prev
        const next = [...prev]
        next[editorIndex] = result.imageFile
        if (result.maskFile) {
          next.splice(editorIndex + 1, 0, result.maskFile)
        }
        return next
      })
      closeEditor()
    },
    [closeEditor, editorIndex],
  )

  // Helper to update message and sync to parent
  const updateMessage = useCallback(
    (newValue: string | ((prev: string) => string)) => {
      setMessage((prev) => {
        const next = typeof newValue === 'function' ? newValue(prev) : newValue
        if (onDraftChange && next !== prev) {
          onDraftChange(next)
        }
        return next
      })
    },
    [onDraftChange],
  )

  // Sync from external draftMessage changes (e.g., switching sessions)
  useEffect(() => {
    if (draftMessage !== undefined && draftMessage !== message) {
      setMessage(draftMessage)
    }
  }, [draftMessage])

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    const maxHeight = 240
    textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`
  }, [message])

  useEffect(() => {
    if (!prefillDraft || prefillDraft.id === lastPrefillIdRef.current) {
      return
    }
    lastPrefillIdRef.current = prefillDraft.id
    updateMessage(prefillDraft.text)
    requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })
    if (onPrefillUsed) {
      onPrefillUsed(prefillDraft.id)
    }
  }, [prefillDraft, onPrefillUsed, updateMessage])

  useEffect(() => {
    return () => {
      if (mediaRecorderRef.current) {
        mediaRecorderRef.current.stream
          .getTracks()
          .forEach((track) => track.stop())
        mediaRecorderRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    if (editorIndex !== null && editorIndex >= attachedFiles.length) {
      setEditorIndex(null)
      setIsEditorOpen(false)
    }
  }, [attachedFiles.length, editorIndex])

  // Auto-focus when input becomes enabled or session/model/config changes
  useEffect(() => {
    if (!disabled && !isLoading && !isTranscribing && !isSidebarOpen) {
      // Use setTimeout to ensure focus is applied after any other focus
      // management (like Radix UI dropdown focus restoration)
      const timer = setTimeout(() => {
        textareaRef.current?.focus()
      }, 50)
      return () => clearTimeout(timer)
    }
  }, [
    disabled,
    isLoading,
    isTranscribing,
    isSidebarOpen,
    sessionId,
    config,
    draftMessage,
  ])

  const handleSend = useCallback(() => {
    const trimmed = String(message || '').trim()
    if (!trimmed || disabled || isLoading || isTranscribing) return
    const payload = trimmed
    onSend(payload, attachedFiles.length > 0 ? attachedFiles : undefined)
    updateMessage('')
    setAttachedFiles([])
  }, [
    message,
    attachedFiles,
    disabled,
    isLoading,
    isTranscribing,
    onSend,
    updateMessage,
  ])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // Send on Ctrl+Enter or Cmd+Enter
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSend()
        return
      }
    },
    [handleSend],
  )

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (files) {
        appendFiles(Array.from(files))
      }
      // Reset input
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
    },
    [appendFiles],
  )

  const removeFile = useCallback((index: number) => {
    setAttachedFiles((prev) => prev.filter((_, i) => i !== index))
    setEditorIndex((prev) => {
      if (prev === null) return prev
      if (prev === index) {
        setIsEditorOpen(false)
        return null
      }
      if (prev > index) {
        return prev - 1
      }
      return prev
    })
  }, [])

  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const items = Array.from(e.clipboardData?.items || [])
      const files: File[] = []
      items.forEach((item) => {
        if (item.kind === 'file') {
          const file = item.getAsFile()
          if (file) {
            files.push(file)
          }
        }
      })
      if (files.length > 0) {
        appendFiles(files)
      }
    },
    [appendFiles],
  )

  const handleDrop = useCallback(
    (e: React.DragEvent<HTMLDivElement>) => {
      e.preventDefault()
      if (e.dataTransfer?.files?.length) {
        appendFiles(Array.from(e.dataTransfer.files))
      }
    },
    [appendFiles],
  )

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
  }, [])

  const toggleSwitch = useCallback(
    (key: keyof SessionConfig['chat_switch']) => {
      if (onConfigChange) {
        const currentValue = config.chat_switch[key]
        if (typeof currentValue === 'boolean') {
          onConfigChange({ [key]: !currentValue })
        }
      }
    },
    [config.chat_switch, onConfigChange],
  )

  const transcribeBlob = useCallback(
    async (blob: Blob) => {
      if (!config.api_token) {
        alert('API token is required for voice transcription.')
        return
      }
      setIsTranscribing(true)
      try {
        const file = new File([blob], `voice-${Date.now()}.webm`, {
          type: blob.type || 'audio/webm',
        })
        const text = await transcribeAudio(file, config.api_token)
        updateMessage((prev) => (prev ? `${prev}\n${text}` : text))
      } catch (err) {
        console.error('Failed to transcribe audio:', err)
        alert(
          'Failed to transcribe audio. Please check the console for details.',
        )
      } finally {
        setIsTranscribing(false)
      }
    },
    [config.api_base, config.api_token],
  )

  const stopRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current
    if (!recorder) return
    recorder.stop()
    recorder.stream.getTracks().forEach((track) => track.stop())
    mediaRecorderRef.current = null
    setIsRecording(false)
  }, [])

  const startRecording = useCallback(async () => {
    if (isRecording) return
    if (!navigator.mediaDevices?.getUserMedia) {
      alert('Your browser does not support audio recording.')
      return
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const recorder = new MediaRecorder(stream)
      mediaRecorderRef.current = recorder
      recordedChunksRef.current = []
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          recordedChunksRef.current.push(event.data)
        }
      }
      recorder.onstop = () => {
        const blob = new Blob(recordedChunksRef.current, {
          type: recorder.mimeType || 'audio/webm',
        })
        recordedChunksRef.current = []
        void transcribeBlob(blob)
      }
      recorder.start()
      setIsRecording(true)
    } catch (err) {
      console.error('Unable to access microphone:', err)
      alert('Unable to access microphone. Please check permissions.')
    }
  }, [isRecording, transcribeBlob])

  const handleToggleRecording = useCallback(() => {
    if (isRecording) {
      stopRecording()
    } else {
      startRecording()
    }
  }, [isRecording, startRecording, stopRecording])

  const editorFile = editorIndex !== null ? attachedFiles[editorIndex] : null

  return (
    <>
      <div
        className="theme-surface w-full p-1"
        onDragOver={handleDragOver}
        onDrop={handleDrop}
      >
        {attachedFiles.length > 0 && (
          <div className="mb-1 flex flex-wrap gap-1">
            {attachedFiles.map((file, index) => (
              <FileItem
                key={`${file.name}-${index}`}
                file={file}
                index={index}
                onRemove={removeFile}
                onEdit={openEditorForIndex}
              />
            ))}
          </div>
        )}

        <div className="flex items-start gap-1.5">
          <div className="relative flex-1">
            <Textarea
              ref={textareaRef}
              value={message}
              onChange={(e) => updateMessage(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              placeholder={placeholder}
              disabled={disabled || isLoading || isTranscribing}
              className="min-h-[80px] w-full resize-none rounded-md border-0 bg-transparent px-0 pr-16 pb-1.5 text-base shadow-none ring-0 focus:ring-0 dark:bg-transparent"
              rows={3}
            />
            <span className="pointer-events-none absolute bottom-0 left-0 text-[9px] text-muted-foreground/50">
              Ctrl+Enter to send
            </span>
          </div>

          <div className="flex shrink-0 items-center gap-1">
            {config.chat_switch.enable_talk && (
              <Button
                type="button"
                onClick={handleToggleRecording}
                disabled={disabled || isLoading || isTranscribing}
                variant={isRecording ? 'destructive' : 'outline'}
                className="h-9 w-9 rounded-md p-0 shadow-sm"
              >
                {isRecording ? (
                  <Square className="h-4 w-4" />
                ) : isTranscribing ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Mic className="h-4 w-4" />
                )}
              </Button>
            )}

            {isLoading ? (
              <Button
                onClick={onStop}
                variant="destructive"
                className="h-9 w-9 rounded-md p-0 shadow-sm"
              >
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button
                onClick={handleSend}
                disabled={
                  !String(message || '').trim() || disabled || isTranscribing
                }
                className="h-9 rounded-md bg-primary px-3 text-sm font-semibold text-primary-foreground shadow-md transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:bg-primary/20"
              >
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>

        <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
          <ToggleButton
            active={!config.chat_switch.disable_https_crawler}
            onClick={() => toggleSwitch('disable_https_crawler')}
            icon={<Link className="h-3 w-3" />}
            label="URL Fetch"
            title="Automatically fetch content from URLs in your message"
          />

          <ToggleButton
            active={config.chat_switch.enable_mcp}
            onClick={() => toggleSwitch('enable_mcp')}
            icon={<span className="text-xs">🔧</span>}
            label="MCP"
            title="Enable MCP tools"
          />

          <ToggleButton
            active={config.chat_switch.all_in_one}
            onClick={() => toggleSwitch('all_in_one')}
            icon={<Image className="h-3 w-3" />}
            label="Draw"
            title="Combine chat and image generation"
          />

          <ToggleButton
            active={config.chat_switch.enable_talk}
            onClick={() => toggleSwitch('enable_talk')}
            icon={<Mic className="h-3 w-3" />}
            label="Voice"
            title="Enable voice mode"
          />

          <div className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept="image/*,.pdf,.doc,.docx,.txt,.md"
              onChange={handleFileSelect}
              className="hidden"
            />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => fileInputRef.current?.click()}
              disabled={disabled || isLoading}
              className="h-7 w-7 rounded-md p-0 text-foreground hover:bg-muted"
              title="Attach file"
            >
              <Paperclip className="h-3.5 w-3.5" />
            </Button>
            {isTranscribing && !isRecording && (
              <span className="text-primary">Transcribing…</span>
            )}
          </div>
        </div>
      </div>
      <ImageEditorModal
        open={isEditorOpen}
        file={editorFile}
        onClose={closeEditor}
        onSave={handleEditorSave}
      />
    </>
  )
}

interface ToggleButtonProps {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
  title: string
}

function ToggleButton({
  active,
  onClick,
  icon,
  label,
  title,
}: ToggleButtonProps) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={cn(
        'flex items-center gap-1 rounded-md px-2.5 py-1.5 text-[11px] transition-colors',
        active
          ? 'bg-primary/10 text-primary ring-1 ring-primary/20'
          : 'bg-muted text-muted-foreground hover:bg-muted/80',
      )}
    >
      {icon}
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}
