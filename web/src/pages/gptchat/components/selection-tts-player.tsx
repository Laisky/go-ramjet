/**
 * SelectionTTSPlayer renders a unified audio player for selected text.
 */
import { Loader2, X } from 'lucide-react'
import { useEffect, useRef } from 'react'

import { Button } from '@/components/ui/button'
import { TooltipWrapper } from '@/components/ui/tooltip-wrapper'
import { cn } from '@/utils/cn'

export interface SelectionTTSPlayerProps {
  /** URL of the audio to play */
  audioUrl: string | null
  /** Whether the player is loading audio */
  isLoading: boolean
  /** Error message to display */
  error: string | null
  /** Called when the user closes the player */
  onClose: () => void
  /** Optional extra class names */
  className?: string
}

/**
 * SelectionTTSPlayer displays a fixed, themed audio player for selections.
 */
export function SelectionTTSPlayer({
  audioUrl,
  isLoading,
  error,
  onClose,
  className,
}: SelectionTTSPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null)

  useEffect(() => {
    const audio = audioRef.current
    if (!audio || !audioUrl) {
      return
    }
    audio.play().catch((err) => {
      console.debug('[SelectionTTSPlayer] Autoplay blocked:', err)
    })
  }, [audioUrl])

  if (!audioUrl && !isLoading && !error) {
    return null
  }

  return (
    <div
      className={cn(
        'resize-x overflow-auto rounded-xl border border-border/60 bg-card/95 px-3 py-2 shadow-lg backdrop-blur',
        className,
      )}
      style={{
        width: 'min(720px, 96vw)',
        minWidth: '280px',
        maxWidth: '96vw',
      }}
    >
      <div className="mb-2 flex items-center justify-between text-[11px] text-muted-foreground">
        <span className="font-medium text-foreground/80">Selection audio</span>
        <TooltipWrapper content="Close audio player">
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            className="h-6 w-6 rounded-md p-0"
            aria-label="Close audio player"
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </TooltipWrapper>
      </div>

      {isLoading && (
        <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Preparing audio…</span>
        </div>
      )}

      {error && (
        <div className="mb-2 rounded-md bg-destructive/10 px-2 py-1 text-xs text-destructive">
          {error}
        </div>
      )}

      {audioUrl && (
        <audio
          ref={audioRef}
          src={audioUrl}
          controls
          className="h-9 w-full"
          playsInline
          preload="auto"
        />
      )}
    </div>
  )
}
