/**
 * Chat input component with file attachments and feature toggles.
 */
import { Send, Square, Paperclip, Link, Image, Mic } from 'lucide-react'
import { useRef, useState, useCallback, type KeyboardEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
import type { SessionConfig } from '../types'

export interface ChatInputProps {
  onSend: (message: string, files?: File[]) => void
  onStop?: () => void
  isLoading?: boolean
  disabled?: boolean
  config: SessionConfig
  onConfigChange?: (updates: Partial<SessionConfig['chat_switch']>) => void
  placeholder?: string
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
}: ChatInputProps) {
  const [message, setMessage] = useState('')
  const [attachedFiles, setAttachedFiles] = useState<File[]>([])
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleSend = useCallback(() => {
    if (!message.trim() || disabled || isLoading) return
    onSend(message.trim(), attachedFiles.length > 0 ? attachedFiles : undefined)
    setMessage('')
    setAttachedFiles([])
  }, [message, attachedFiles, disabled, isLoading, onSend])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // Send on Ctrl+Enter or Cmd+Enter
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSend()
      }
    },
    [handleSend]
  )

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (files) {
        setAttachedFiles((prev) => [...prev, ...Array.from(files)])
      }
      // Reset input
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
    },
    []
  )

  const removeFile = useCallback((index: number) => {
    setAttachedFiles((prev) => prev.filter((_, i) => i !== index))
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
    [config.chat_switch, onConfigChange]
  )

  return (
    <div className="space-y-2">
      {/* Attached files preview */}
      {attachedFiles.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {attachedFiles.map((file, index) => (
            <div
              key={index}
              className="flex items-center gap-1 rounded bg-black/5 px-2 py-1 text-xs dark:bg-white/5"
            >
              <Paperclip className="h-3 w-3" />
              <span className="max-w-[100px] truncate">{file.name}</span>
              <button
                onClick={() => removeFile(index)}
                className="ml-1 text-red-500 hover:text-red-600"
              >
                ×
              </button>
            </div>
          ))}
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
            placeholder={placeholder}
            disabled={disabled || isLoading}
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

        {/* Send/Stop button */}
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
            disabled={!message.trim() || disabled}
            className="h-auto px-4"
          >
            <Send className="h-4 w-4" />
          </Button>
        )}
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

        <span className="ml-auto text-black/40 dark:text-white/40">
          Ctrl+Enter to send
        </span>
      </div>
    </div>
  )
}

interface ToggleButtonProps {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
  title: string
}

function ToggleButton({ active, onClick, icon, label, title }: ToggleButtonProps) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={cn(
        'flex items-center gap-1 rounded-full px-2 py-1 transition-colors',
        active
          ? 'bg-blue-500 text-white'
          : 'bg-black/5 text-black/60 hover:bg-black/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10'
      )}
    >
      {icon}
      <span>{label}</span>
    </button>
  )
}
