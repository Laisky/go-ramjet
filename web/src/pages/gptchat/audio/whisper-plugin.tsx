import { Loader2, Mic, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { transcribeAudio } from '@/utils/api'
import { stopMediaStream } from './media-lifecycle'
import type { AudioPluginProps } from './plugin-types'

/** WhisperAudioPlugin preserves editable dictation and cancels permission/recording work on disposal. */
export function WhisperAudioPlugin({
  config,
  disabled,
  onDraftText,
  onError,
  onBusyChange,
  onActivityChange,
  onStatusChange,
}: AudioPluginProps) {
  const [state, setState] = useState<
    'idle' | 'starting' | 'recording' | 'transcribing'
  >('idle')
  const recorderRef = useRef<MediaRecorder | null>(null)
  const attemptRef = useRef<AbortController | null>(null)
  const mountedRef = useRef(true)

  /** cancel invalidates the attempt before releasing any already acquired media. */
  const cancel = useCallback(() => {
    attemptRef.current?.abort()
    attemptRef.current = null
    const recorder = recorderRef.current
    recorderRef.current = null
    if (recorder) {
      recorder.onstop = null
      recorder.ondataavailable = null
      if (recorder.state !== 'inactive') recorder.stop()
      stopMediaStream(recorder.stream)
    }
    if (mountedRef.current) setState('idle')
    onActivityChange(false)
    onBusyChange(false)
    onStatusChange(null)
  }, [onActivityChange, onBusyChange, onStatusChange])

  /** startRecording creates only one owned recorder and rejects late permission results. */
  const startRecording = useCallback(async () => {
    if (attemptRef.current) return
    if (!config.api_token) {
      onError('API token is required for voice transcription.')
      return
    }
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) {
      onError('Your browser does not support audio recording.')
      return
    }
    const attempt = new AbortController()
    attemptRef.current = attempt
    setState('starting')
    onActivityChange(true)
    onBusyChange(true)
    onStatusChange('Waiting for microphone permission…')
    onError(null)
    let stream: MediaStream | null = null
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      if (attempt.signal.aborted || !mountedRef.current) {
        stopMediaStream(stream)
        return
      }
      const recorder = new MediaRecorder(stream)
      const chunks: Blob[] = []
      recorderRef.current = recorder
      recorder.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data)
      }
      recorder.onstop = () => {
        if (attempt.signal.aborted) return
        recorderRef.current = null
        stopMediaStream(recorder.stream)
        const blob = new Blob(chunks, {
          type: recorder.mimeType || 'audio/webm',
        })
        const extension = blob.type.includes('mp4')
          ? 'mp4'
          : blob.type.includes('ogg')
            ? 'ogg'
            : 'webm'
        setState('transcribing')
        onBusyChange(true)
        onStatusChange('Transcribing…')
        void transcribeAudio(
          new File([blob], `voice-${Date.now()}.${extension}`, {
            type: blob.type,
          }),
          config.api_token,
        )
          .then((text) => {
            if (!attempt.signal.aborted && mountedRef.current) onDraftText(text)
          })
          .catch(() => {
            if (!attempt.signal.aborted && mountedRef.current)
              onError('Failed to transcribe audio. Please try again.')
          })
          .finally(() => {
            if (attemptRef.current === attempt) cancel()
          })
      }
      recorder.start()
      setState('recording')
      onBusyChange(false)
      onStatusChange('Recording…')
    } catch {
      if (stream) stopMediaStream(stream)
      if (!attempt.signal.aborted && mountedRef.current) {
        onError('Unable to access microphone. Please check permissions.')
        cancel()
      }
    }
  }, [
    config.api_token,
    onActivityChange,
    onBusyChange,
    onStatusChange,
    onError,
    onDraftText,
    cancel,
  ])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      cancel()
    }
  }, [cancel])

  return (
    <Button
      type="button"
      onClick={() => {
        if (state === 'starting') cancel()
        else if (state === 'recording') {
          setState('transcribing')
          onBusyChange(true)
          recorderRef.current?.stop()
        } else if (state === 'idle') void startRecording()
      }}
      disabled={state === 'transcribing' || (disabled && state === 'idle')}
      variant={
        state === 'recording' || state === 'starting'
          ? 'destructive'
          : 'outline'
      }
      aria-label={
        state === 'starting'
          ? 'Cancel microphone request'
          : state === 'recording'
            ? 'Stop recording'
            : state === 'transcribing'
              ? 'Transcribing'
              : 'Start Whisper recording'
      }
    >
      {state === 'recording' ? (
        <Square className="h-4 w-4" />
      ) : state === 'starting' || state === 'transcribing' ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <Mic className="h-4 w-4" />
      )}
      {state === 'starting'
        ? 'Cancel'
        : state === 'recording'
          ? 'Stop recording'
          : state === 'transcribing'
            ? 'Transcribing'
            : 'Dictate'}
    </Button>
  )
}
