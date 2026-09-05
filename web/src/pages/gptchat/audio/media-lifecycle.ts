/** abortError creates the cancellation error shared by asynchronous media operations. */
export function abortError(): DOMException {
  return new DOMException('Voice call cancelled', 'AbortError')
}

/** waitForMedia makes an asynchronous operation settle immediately on cancellation. */
export function waitForMedia<T>(
  promise: Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const cancel = () => reject(abortError())
    signal.addEventListener('abort', cancel, { once: true })
    if (signal.aborted) cancel()
    promise
      .then(resolve, reject)
      .finally(() => signal.removeEventListener('abort', cancel))
  })
}

/** stopMediaStream synchronously releases all tracks belonging to the supplied stream. */
export function stopMediaStream(stream: MediaStream): void {
  stream.getTracks().forEach((track) => {
    track.onended = null
    track.stop()
  })
}
