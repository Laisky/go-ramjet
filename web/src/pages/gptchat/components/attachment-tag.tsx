import { cn } from '@/utils/cn'
import { Edit2, Paperclip, X } from 'lucide-react'
import { useEffect, useState } from 'react'

function formatFileSize(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export interface AttachmentTagProps {
  filename: string
  type: 'image' | 'file'
  size?: number
  url?: string
  contentB64?: string
  file?: File // Original File object if available
  onRemove?: () => void
  onEdit?: () => void
  className?: string
}

/**
 * Unified component for displaying attachment tags (images or files).
 */
export function AttachmentTag({
  filename,
  type,
  size,
  url,
  contentB64,
  file,
  onRemove,
  onEdit,
  className,
}: AttachmentTagProps) {
  const [displayUrl, setDisplayUrl] = useState<string | null>(
    url || contentB64 || null,
  )
  const isImage = type === 'image'

  useEffect(() => {
    // If we have a File object but no URL/B64, create an object URL
    if (isImage && file && !url && !contentB64) {
      const objectUrl = URL.createObjectURL(file)
      setDisplayUrl(objectUrl)
      return () => URL.revokeObjectURL(objectUrl)
    }
  }, [file, isImage, url, contentB64])

  // Update displayUrl if url or contentB64 changes
  useEffect(() => {
    if (url || contentB64) {
      setDisplayUrl(url || contentB64 || null)
    }
  }, [url, contentB64])

  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-md border border-border bg-muted px-2 py-1 text-xs shadow-sm',
        className,
      )}
    >
      {isImage && displayUrl ? (
        <div className="h-8 w-8 shrink-0 overflow-hidden rounded border border-border bg-background">
          <img
            src={displayUrl}
            alt={filename}
            className="h-full w-full object-cover"
          />
        </div>
      ) : (
        <Paperclip className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      )}

      <div className="max-w-[180px] truncate">
        <div className="truncate font-medium">{filename}</div>
        {size !== undefined && (
          <div className="text-[10px] text-muted-foreground">
            {formatFileSize(size)}
          </div>
        )}
      </div>

      {isImage && onEdit && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onEdit()
          }}
          className="flex items-center gap-1 rounded-md bg-popover px-1.5 py-0.5 text-[10px] text-popover-foreground shadow-sm transition-colors hover:bg-accent"
          title="Edit image"
        >
          <Edit2 className="h-3 w-3" />
          Edit
        </button>
      )}

      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
          className="ml-1 rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
          title="Remove attachment"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  )
}
