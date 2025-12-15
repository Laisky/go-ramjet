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

const PROMPT_HISTORY_LIMIT = 50

function formatFileSize(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export interface ChatInputProps {
  onSend: (message: string, files?: File[]) => void
  onStop?: () => void
  isLoading?: boolean
  disabled?: boolean
  config: SessionConfig
  onConfigChange?: (updates: Partial<SessionConfig['chat_switch']>) => void
  placeholder?: string
  prefillDraft?: { id: string; text: string }
  onPrefillUsed?: (id: string) => void
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
  onConfigChange,
  placeholder = 'Type a message...',
  prefillDraft,
  onPrefillUsed,
}: ChatInputProps) {
  const [message, setMessage] = useState('')
  const [attachedFiles, setAttachedFiles] = useState<File[]>([])
  const [promptHistory, setPromptHistory] = useState<string[]>([])
  const [historyIndex, setHistoryIndex] = useState<number | null>(null)
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
    setMessage(prefillDraft.text)
    setHistoryIndex(null)
    requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })
    if (onPrefillUsed) {
      onPrefillUsed(prefillDraft.id)
    }
  }, [prefillDraft, onPrefillUsed])

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

  const handleSend = useCallback(() => {
    if (!message.trim() || disabled || isLoading || isTranscribing) return
    const payload = message.trim()
    onSend(payload, attachedFiles.length > 0 ? attachedFiles : undefined)
    setPromptHistory((prev) => [
      ...prev.slice(-(PROMPT_HISTORY_LIMIT - 1)),
      payload,
    ])
    setHistoryIndex(null)
    setMessage('')
    setAttachedFiles([])
  }, [message, attachedFiles, disabled, isLoading, isTranscribing, onSend])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // Send on Ctrl+Enter or Cmd+Enter
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSend()
        return
      }

      if (
        e.key === 'ArrowUp' &&
        !e.shiftKey &&
        !e.altKey &&
        !e.ctrlKey &&
        !e.metaKey
      ) {
        const textarea = textareaRef.current
        if (
          textarea &&
          textarea.selectionStart === 0 &&
          textarea.selectionEnd === 0
        ) {
          e.preventDefault()
          if (promptHistory.length === 0) return
          const nextIndex =
            historyIndex === null
              ? promptHistory.length - 1
              : Math.max(historyIndex - 1, 0)
          setHistoryIndex(nextIndex)
          const nextValue = promptHistory[nextIndex] ?? ''
          setMessage(nextValue)
          requestAnimationFrame(() => {
            textarea.selectionStart = textarea.selectionEnd = 0
          })
        }
        return
      }

      if (
        e.key === 'ArrowDown' &&
        !e.shiftKey &&
        !e.altKey &&
        !e.ctrlKey &&
        !e.metaKey
      ) {
        const textarea = textareaRef.current
        if (
          textarea &&
          textarea.selectionStart === message.length &&
          textarea.selectionEnd === message.length
        ) {
          e.preventDefault()
          if (historyIndex === null) {
            setMessage('')
            return
          }

          const nextIndex = historyIndex + 1
          if (nextIndex >= promptHistory.length) {
            setHistoryIndex(null)
            setMessage('')
          } else {
            setHistoryIndex(nextIndex)
            const nextValue = promptHistory[nextIndex] ?? ''
            setMessage(nextValue)
            requestAnimationFrame(() => {
              textarea.selectionStart = textarea.selectionEnd = nextValue.length
            })
          }
        }
      }
    },
    [handleSend, historyIndex, message.length, promptHistory],
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
        setMessage((prev) => (prev ? `${prev}\n${text}` : text))
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
        className="space-y-2"
        onDragOver={handleDragOver}
        onDrop={handleDrop}
      >
        {/* Attached files preview */}
        {attachedFiles.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {attachedFiles.map((file, index) => {
              const isImage = file.type.startsWith('image/')
              return (
                <div
                  key={`${file.name}-${index}`}
                  className="flex items-center gap-2 rounded border border-black/10 bg-black/5 px-2 py-1 text-xs dark:border-white/10 dark:bg-white/5"
                >
                  <Paperclip className="h-3 w-3" />
                  <div className="max-w-[140px] truncate">
                    <div className="truncate font-medium">{file.name}</div>
                    <div className="text-[10px] text-black/60 dark:text-white/60">
                      {formatFileSize(file.size)}
                    </div>
                  </div>
                  {isImage && (
                    <button
                      type="button"
                      onClick={() => openEditorForIndex(index)}
                      className="flex items-center gap-1 rounded bg-white/80 px-1.5 py-0.5 text-[10px] text-black shadow dark:bg-black/60 dark:text-white"
                    >
                      <Edit2 className="h-3 w-3" />
                      Edit
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => removeFile(index)}
                    className="text-red-500 hover:text-red-600"
                  >
                    ×
                  </button>
                </div>
              )
            })}
          </div>
        )}

        {/* Main input area */}
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Textarea
              ref={textareaRef}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              placeholder={placeholder}
              disabled={disabled || isLoading || isTranscribing}
              className="min-h-[60px] resize-none pr-10"
              rows={2}
            />

            {/* File attachment button */}
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
              className="absolute bottom-2 right-2 h-6 w-6 p-0"
            >
              <Paperclip className="h-4 w-4" />
            </Button>
          </div>

          <div className="flex flex-col gap-2">
            {config.chat_switch.enable_talk && (
              <Button
                type="button"
                onClick={handleToggleRecording}
                disabled={disabled || isLoading || isTranscribing}
                variant={isRecording ? 'destructive' : 'outline'}
                className="h-auto px-3"
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
                className="h-auto px-4"
              >
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button
                onClick={handleSend}
                disabled={!message.trim() || disabled || isTranscribing}
                className="h-auto px-4"
              >
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>

        {/* Feature toggles */}
        <div className="flex flex-wrap items-center gap-2 text-xs">
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

          <div className="ml-auto flex gap-3 text-black/40 dark:text-white/40">
            {isTranscribing && !isRecording && (
              <span className="text-blue-500 dark:text-blue-300">
                Transcribing audio…
              </span>
            )}
            <span>Ctrl+Enter to send</span>
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
        'flex items-center gap-1 rounded-full px-2 py-1 transition-colors',
        active
          ? 'bg-blue-500 text-white'
          : 'bg-black/5 text-black/60 hover:bg-black/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10',
      )}
    >
      {icon}
      <span>{label}</span>
    </button>
  )
}
