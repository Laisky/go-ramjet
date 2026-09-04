import { Loader2, Mic, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { transcribeAudio } from '@/utils/api'
import type { AudioPluginProps } from './plugin-types'

/** WhisperAudioPlugin preserves the legacy record-then-transcribe interaction. */
export function WhisperAudioPlugin({
  config,
  disabled,
  onDraftText,
  onError,
  onBusyChange,
  onActivityChange,
  onStatusChange,
}: AudioPluginProps) {
  const [isRecording, setIsRecording] = useState(false)
  const [isTranscribing, setIsTranscribing] = useState(false)
  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const mountedRef = useRef(true)

  const transcribeBlob = useCallback(
    async (blob: Blob) => {
      if (!config.api_token) {
        onError('API token is required for voice transcription.')
        return
      }

      setIsTranscribing(true)
      onBusyChange(true)
      onActivityChange(true)
      onStatusChange('Transcribing…')
      onError(null)
      try {
        const file = new File([blob], `voice-${Date.now()}.webm`, {
          type: blob.type || 'audio/webm',
        })
        const text = await transcribeAudio(file, config.api_token)
        if (mountedRef.current) {
          onDraftText(text)
        }
      } catch {
        console.error('Failed to transcribe audio')
        if (mountedRef.current) {
          onError('Failed to transcribe audio. Please try again.')
        }
      } finally {
        if (mountedRef.current) {
          setIsTranscribing(false)
          onBusyChange(false)
          onActivityChange(false)
          onStatusChange(null)
        }
      }
    },
    [
      config.api_token,
      onActivityChange,
      onBusyChange,
      onDraftText,
      onError,
      onStatusChange,
    ],
  )

  const stopRecording = useCallback(() => {
    const recorder = recorderRef.current
    if (!recorder) {
      return
    }

    if (recorder.state !== 'inactive') {
      recorder.stop()
    }
    recorder.stream.getTracks().forEach((track) => track.stop())
    recorderRef.current = null
    setIsRecording(false)
    onStatusChange('Transcribing…')
  }, [onStatusChange])

  const startRecording = useCallback(async () => {
    if (isRecording) {
      return
    }
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) {
      onError('Your browser does not support audio recording.')
      return
    }

    onError(null)
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const recorder = new MediaRecorder(stream)
      recorderRef.current = recorder
      chunksRef.current = []
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          chunksRef.current.push(event.data)
        }
      }
      recorder.onstop = () => {
        const blob = new Blob(chunksRef.current, {
          type: recorder.mimeType || 'audio/webm',
        })
        chunksRef.current = []
        if (mountedRef.current) {
          void transcribeBlob(blob)
        }
      }
      recorder.start()
      setIsRecording(true)
      onActivityChange(true)
      onStatusChange('Recording…')
    } catch {
      console.error('Unable to access microphone')
      onError('Unable to access microphone. Please check permissions.')
      onActivityChange(false)
      onStatusChange(null)
    }
  }, [isRecording, onActivityChange, onError, onStatusChange, transcribeBlob])

  const handleToggle = useCallback(() => {
    if (isRecording) {
      stopRecording()
      return
    }
    void startRecording()
  }, [isRecording, startRecording, stopRecording])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      const recorder = recorderRef.current
      recorderRef.current = null
      if (recorder) {
        recorder.onstop = null
        if (recorder.state !== 'inactive') {
          recorder.stop()
        }
        recorder.stream.getTracks().forEach((track) => track.stop())
      }
      chunksRef.current = []
      onBusyChange(false)
      onActivityChange(false)
      onStatusChange(null)
    }
  }, [onActivityChange, onBusyChange, onStatusChange])

  return (
    <Button
      type="button"
      onClick={handleToggle}
      disabled={isTranscribing || (disabled && !isRecording)}
      variant={isRecording ? 'destructive' : 'outline'}
      className="w-10 rounded-md p-0 shadow-sm"
      aria-label={
        isRecording
          ? 'Stop recording'
          : isTranscribing
            ? 'Transcribing'
            : 'Start Whisper recording'
      }
    >
      {isRecording ? (
        <Square className="h-5 w-5" />
      ) : isTranscribing ? (
        <Loader2 className="h-5 w-5 animate-spin" />
      ) : (
        <Mic className="h-5 w-5" />
      )}
    </Button>
  )
}
