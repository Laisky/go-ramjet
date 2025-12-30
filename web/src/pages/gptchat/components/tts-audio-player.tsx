/**
 * TTS Audio Player component for displaying audio controls.
 *
 * This component is used by the ChatMessage component to display
 * an inline audio player for TTS playback.
 */
import { X } from 'lucide-react'
import { useEffect, useRef } from 'react'

import { Button } from '@/components/ui/button'

export interface TTSAudioPlayerProps {
  /** URL of the audio to play */
  audioUrl: string
  /** Callback when the close button is clicked */
  onClose: () => void
  /** Callback ref setter for the audio element */
  setAudioRef?: (element: HTMLAudioElement | null) => void
}

/**
 * TTSAudioPlayer renders an audio player with controls and a close button.
 */
export function TTSAudioPlayer({
  audioUrl,
  onClose,
  setAudioRef,
}: TTSAudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null)

  // Connect the ref to the parent's setAudioRef callback
  useEffect(() => {
    if (setAudioRef) {
      setAudioRef(audioRef.current)
    }

    return () => {
      if (setAudioRef) {
        setAudioRef(null)
      }
    }
  }, [setAudioRef])

  // Try autoplay when mounted
  useEffect(() => {
    const audio = audioRef.current
    if (audio && audioUrl) {
      audio.play().catch((err) => {
        console.debug(
          '[TTSAudioPlayer] Autoplay blocked (expected on mobile):',
          err,
        )
      })
    }
  }, [audioUrl])

  return (
    <div className="mt-2 flex items-center gap-2 rounded-md bg-muted/50 p-2">
      <audio
        ref={audioRef}
        src={audioUrl}
        controls
        className="h-8 w-full max-w-xs"
        playsInline
        preload="auto"
      />
      <Button
        variant="ghost"
        size="sm"
        onClick={onClose}
        className="h-7 w-7 shrink-0 rounded-md p-0"
        title="Close audio player"
      >
        <X className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}
