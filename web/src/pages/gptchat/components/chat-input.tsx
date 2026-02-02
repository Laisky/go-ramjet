/**
 * Chat input component with file attachments and feature toggles.
 */
import { Image, Link, Loader2, Mic, Send, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { transcribeAudio } from '@/utils/api'
import { cn } from '@/utils/cn'
import { useUser } from '../hooks/use-user'
import { isImageModel } from '../models'
import type { ChatAttachment, SelectionData, SessionConfig } from '../types'
import { MessageInput } from './message-input'

export interface ChatInputProps {
  onSend: (message: string, attachments?: ChatAttachment[]) => void
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
  onSelectionChange?: (selection: SelectionData | null) => void
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
  onSelectionChange,
}: ChatInputProps) {
  const { user } = useUser(config.api_token)
  const isFree = user?.is_free ?? true
  const [message, setMessage] = useState(() => draftMessage ?? '')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [isRecording, setIsRecording] = useState(false)
  const [isTranscribing, setIsTranscribing] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const recordedChunksRef = useRef<Blob[]>([])
  const lastPrefillIdRef = useRef<string | null>(null)
  const lastPointerRef = useRef<{ x: number; y: number } | null>(null)

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
    onSend(trimmed, attachments.length > 0 ? attachments : undefined)
    updateMessage('')
    setAttachments([])
  }, [
    message,
    attachments,
    disabled,
    isLoading,
    isTranscribing,
    onSend,
    updateMessage,
  ])

  /**
   * emitInputSelection reports a text selection in the textarea to the parent.
   */
  const emitInputSelection = useCallback(
    (position?: { top: number; left: number }) => {
      if (!onSelectionChange) return
      const textarea = textareaRef.current
      if (!textarea) return
      const start = textarea.selectionStart ?? 0
      const end = textarea.selectionEnd ?? 0
      if (start === end) {
        onSelectionChange(null)
        return
      }

      const text = textarea.value.slice(start, end)
      if (!text.trim()) {
        onSelectionChange(null)
        return
      }

      const rect = textarea.getBoundingClientRect()
      const fallbackPosition = {
        top: rect.top + 8,
        left: rect.left + rect.width / 2,
      }

      onSelectionChange({
        text,
        copyText: text,
        source: 'input',
        position: position ?? fallbackPosition,
      })
    },
    [onSelectionChange],
  )

  /**
   * handleInputMouseUp captures pointer coordinates and emits selection data.
   */
  const handleInputMouseUp = useCallback(
    (e: React.MouseEvent<HTMLTextAreaElement>) => {
      lastPointerRef.current = { x: e.clientX, y: e.clientY }
      emitInputSelection({ top: e.clientY, left: e.clientX })
    },
    [emitInputSelection],
  )

  /**
   * handleInputKeyUp emits selection data for keyboard-based selections.
   */
  const handleInputKeyUp = useCallback(() => {
    const lastPointer = lastPointerRef.current
    emitInputSelection(
      lastPointer ? { top: lastPointer.y, left: lastPointer.x } : undefined,
    )
  }, [emitInputSelection])

  /**
   * handleInputSelect emits selection data when the textarea selection changes.
   */
  const handleInputSelect = useCallback(() => {
    const lastPointer = lastPointerRef.current
    emitInputSelection(
      lastPointer ? { top: lastPointer.y, left: lastPointer.x } : undefined,
    )
  }, [emitInputSelection])

  /**
   * handleInputBlur clears selection data when the textarea loses focus.
   */
  const handleInputBlur = useCallback(() => {
    if (onSelectionChange) {
      onSelectionChange(null)
    }
  }, [onSelectionChange])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // Ignore keyboard events when composition is in progress (IME)
      if (e.nativeEvent.isComposing) return

      // Send on Ctrl+Enter or Cmd+Enter
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSend()
        return
      }
    },
    [handleSend],
  )

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

  return (
    <>
      <div className="theme-surface w-full p-1">
        <div className="flex items-start gap-1.5">
          <MessageInput
            textareaRef={textareaRef}
            value={message}
            onChange={updateMessage}
            attachments={attachments}
            onAttachmentsChange={setAttachments}
            onKeyDown={handleKeyDown}
            onKeyUp={handleInputKeyUp}
            onMouseUp={handleInputMouseUp}
            onSelect={handleInputSelect}
            onBlur={handleInputBlur}
            placeholder={placeholder}
            disabled={disabled || isLoading || isTranscribing}
            apiToken={config.api_token}
            className="flex-1"
          />

          <div className="flex shrink-0 self-stretch items-stretch gap-1">
            <div className="flex flex-col gap-1">
              {isLoading ? (
                <Button
                  onClick={onStop}
                  variant="destructive"
                  className="flex-1 w-12 rounded-md p-0 shadow-sm"
                  aria-label="Stop generation"
                >
                  <Square className="h-5 w-5" />
                </Button>
              ) : (
                <Button
                  onClick={handleSend}
                  disabled={
                    !String(message || '').trim() || disabled || isTranscribing
                  }
                  className="flex-1 w-12 rounded-md bg-primary p-0 text-primary-foreground shadow-md transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:bg-primary/20"
                  aria-label="Send message"
                  title="Send message (Ctrl+Enter)"
                >
                  <Send className="h-5 w-5" />
                </Button>
              )}
            </div>

            {config.chat_switch.enable_talk && (
              <Button
                type="button"
                onClick={handleToggleRecording}
                disabled={disabled || isLoading || isTranscribing}
                variant={isRecording ? 'destructive' : 'outline'}
                className="w-10 rounded-md p-0 shadow-sm"
              >
                {isRecording ? (
                  <Square className="h-5 w-5" />
                ) : isTranscribing ? (
                  <Loader2 className="h-5 w-5 animate-spin" />
                ) : (
                  <Mic className="h-5 w-5" />
                )}
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

          {(isImageModel(config.selected_model) ||
            config.chat_switch.all_in_one) && (
            <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1">
              <span className="text-[10px] sm:text-[11px] font-medium">
                Images:
              </span>
              <select
                value={config.chat_switch.draw_n_images}
                onChange={(e) =>
                  onConfigChange?.({
                    draw_n_images: parseInt(e.target.value, 10),
                  })
                }
                disabled={isFree}
                className="cursor-pointer bg-transparent text-[11px] focus:outline-none disabled:cursor-not-allowed"
              >
                {[1, 2, 3, 4, 5, 6, 7, 8].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
            {isTranscribing && !isRecording && (
              <span className="text-primary">Transcribing…</span>
            )}
          </div>
        </div>
      </div>
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
      role="switch"
      aria-checked={active}
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
