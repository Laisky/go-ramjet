import * as Dialog from '@radix-ui/react-dialog'
import { Eraser, Paintbrush, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'

export interface ImageEditorResult {
  imageFile: File
  maskFile?: File
}

interface ImageEditorModalProps {
  open: boolean
  file?: File | null
  onClose: () => void
  onSave: (result: ImageEditorResult) => void
}

const MAX_CANVAS_EDGE = 768

function canvasToFile(
  canvas: HTMLCanvasElement,
  filename: string,
): Promise<File | null> {
  return new Promise((resolve) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        resolve(null)
        return
      }
      resolve(new File([blob], filename, { type: 'image/png' }))
    }, 'image/png')
  })
}

function getBaseName(name?: string): string {
  if (!name) {
    return 'image'
  }
  const idx = name.lastIndexOf('.')
  if (idx === -1) {
    return name
  }
  return name.slice(0, idx)
}

export function ImageEditorModal({
  open,
  file,
  onClose,
  onSave,
}: ImageEditorModalProps) {
  const baseCanvasRef = useRef<HTMLCanvasElement>(null)
  const maskCanvasRef = useRef<HTMLCanvasElement>(null)
  const [imageUrl, setImageUrl] = useState<string | null>(null)
  const [loadedImage, setLoadedImage] = useState<HTMLImageElement | null>(null)
  const [brushSize, setBrushSize] = useState(32)
  const [isErasing, setIsErasing] = useState(false)
  const [hasMask, setHasMask] = useState(false)
  const drawingRef = useRef<{ drawing: boolean; x: number; y: number }>({
    drawing: false,
    x: 0,
    y: 0,
  })

  useEffect(() => {
    if (!file) {
      setImageUrl(null)
      return
    }
    const url = URL.createObjectURL(file)
    setImageUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [file])

  useEffect(() => {
    if (!open || !imageUrl) {
      setLoadedImage(null)
      return
    }

    const img = new Image()
    img.onload = () => setLoadedImage(img)
    img.src = imageUrl
  }, [imageUrl, open])

  useEffect(() => {
    const baseCanvas = baseCanvasRef.current
    const maskCanvas = maskCanvasRef.current
    if (!open || !loadedImage || !baseCanvas || !maskCanvas) {
      return
    }

    const img = loadedImage
    const scale = Math.min(
      MAX_CANVAS_EDGE / img.width,
      MAX_CANVAS_EDGE / img.height,
      1,
    )
    const width = Math.max(Math.round(img.width * scale), 1)
    const height = Math.max(Math.round(img.height * scale), 1)

    baseCanvas.width = width
    baseCanvas.height = height
    maskCanvas.width = width
    maskCanvas.height = height

    const ctx = baseCanvas.getContext('2d')
    ctx?.clearRect(0, 0, width, height)
    ctx?.drawImage(img, 0, 0, width, height)

    const maskCtx = maskCanvas.getContext('2d')
    maskCtx?.clearRect(0, 0, width, height)
    setHasMask(false)
  }, [open, loadedImage])

  const pointerToCanvas = useCallback(
    (event: React.PointerEvent<HTMLCanvasElement>) => {
      const canvas = maskCanvasRef.current
      if (!canvas) return { x: 0, y: 0 }
      const rect = canvas.getBoundingClientRect()
      return {
        x: ((event.clientX - rect.left) / rect.width) * canvas.width,
        y: ((event.clientY - rect.top) / rect.height) * canvas.height,
      }
    },
    [],
  )

  const drawStroke = useCallback(
    (x: number, y: number, isNew: boolean) => {
      const canvas = maskCanvasRef.current
      if (!canvas) return
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      ctx.lineWidth = brushSize
      ctx.strokeStyle = isErasing ? 'rgba(0,0,0,1)' : 'rgba(255,0,0,0.85)'
      ctx.globalCompositeOperation = isErasing
        ? 'destination-out'
        : 'source-over'

      if (isNew) {
        ctx.beginPath()
        ctx.moveTo(x, y)
      } else {
        ctx.lineTo(x, y)
        ctx.stroke()
      }
    },
    [brushSize, isErasing],
  )

  const handlePointerDown = useCallback(
    (event: React.PointerEvent<HTMLCanvasElement>) => {
      event.preventDefault()
      const { x, y } = pointerToCanvas(event)
      drawingRef.current = { drawing: true, x, y }
      drawStroke(x, y, true)
      if (!isErasing) {
        setHasMask(true)
      }
    },
    [drawStroke, pointerToCanvas, isErasing],
  )

  const handlePointerMove = useCallback(
    (event: React.PointerEvent<HTMLCanvasElement>) => {
      if (!drawingRef.current.drawing) return
      const { x, y } = pointerToCanvas(event)
      drawStroke(x, y, false)
      drawingRef.current = { drawing: true, x, y }
    },
    [drawStroke, pointerToCanvas],
  )

  const stopDrawing = useCallback(() => {
    if (drawingRef.current.drawing) {
      drawingRef.current = { drawing: false, x: 0, y: 0 }
    }
  }, [])

  useEffect(() => {
    if (!open) {
      stopDrawing()
    }
  }, [open, stopDrawing])

  const clearMask = useCallback(() => {
    const canvas = maskCanvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    setHasMask(false)
  }, [])

  const handleSave = useCallback(async () => {
    if (!file || !baseCanvasRef.current) return
    const baseName = getBaseName(file.name)
    const imageFile = await canvasToFile(
      baseCanvasRef.current,
      `${baseName}-edit.png`,
    )
    if (!imageFile) return

    let maskFile: File | undefined
    if (hasMask && maskCanvasRef.current) {
      maskFile =
        (await canvasToFile(maskCanvasRef.current, `${baseName}-mask.png`)) ??
        undefined
    }

    onSave({ imageFile, maskFile })
    onClose()
  }, [file, hasMask, onClose, onSave])

  const canEdit = useMemo(() => Boolean(file && imageUrl), [file, imageUrl])

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-background/80 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 flex w-[90vw] max-w-4xl -translate-x-1/2 -translate-y-1/2 flex-col gap-4 rounded-lg bg-background p-4 shadow-2xl border border-border">
          <div className="flex items-center justify-between">
            <Dialog.Title className="text-lg font-semibold">
              Image Editor
            </Dialog.Title>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              onClick={onClose}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          {!canEdit && (
            <p className="text-sm text-muted-foreground">
              Select an image to begin editing.
            </p>
          )}

          {canEdit && (
            <div className="flex flex-col gap-4">
              <div className="relative mx-auto max-h-[70vh] max-w-full overflow-auto rounded-lg border border-border bg-muted p-2">
                <div className="relative inline-block">
                  <canvas ref={baseCanvasRef} className="block" />
                  <canvas
                    ref={maskCanvasRef}
                    className="absolute left-0 top-0 cursor-crosshair"
                    onPointerDown={handlePointerDown}
                    onPointerMove={handlePointerMove}
                    onPointerUp={stopDrawing}
                    onPointerLeave={stopDrawing}
                  />
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-3 text-sm">
                <label className="flex items-center gap-2">
                  Brush Size
                  <input
                    type="range"
                    min={4}
                    max={120}
                    step={2}
                    value={brushSize}
                    onChange={(event) =>
                      setBrushSize(Number(event.target.value))
                    }
                  />
                  <span className="w-10 text-right text-xs">{brushSize}px</span>
                </label>

                <Button
                  variant={isErasing ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setIsErasing((prev) => !prev)}
                  className="flex items-center gap-2"
                >
                  {isErasing ? (
                    <Eraser className="h-4 w-4" />
                  ) : (
                    <Paintbrush className="h-4 w-4" />
                  )}
                  {isErasing ? 'Erase Mask' : 'Paint Mask'}
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={clearMask}
                  disabled={!hasMask}
                >
                  Clear Mask
                </Button>

                {hasMask && (
                  <span className="text-xs text-success">Mask ready</span>
                )}
              </div>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={!canEdit}>
              Save Edit
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
