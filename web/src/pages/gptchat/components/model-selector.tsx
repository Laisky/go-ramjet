/**
 * Model selector dropdown component.
 */
import { ChevronDown } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/utils/cn'
import {
  ModelCategories,
  getFirstAllowedModel,
  isFreeModel,
  isModelAllowed,
} from '../models'

/**
 * ModelSelectorProps describes the configuration for rendering a model picker dropdown.
 */
export interface ModelSelectorProps {
  selectedModel: string
  onModelChange: (model: string) => void
  disabled?: boolean
  allowedModels?: string[]
  label?: string
  categories?: string[]
  active?: boolean
  className?: string
  compact?: boolean
  tone?: 'default' | 'ghost'
}

/**
 * ModelSelector provides a categorized model selection dropdown.
 */
export function ModelSelector({
  selectedModel,
  onModelChange,
  disabled,
  allowedModels,
  label,
  categories,
  active,
  className,
  compact,
  tone = 'default',
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)

  const filteredCategories = categories?.length
    ? Object.entries(ModelCategories).filter(([category]) =>
        categories.includes(category),
      )
    : Object.entries(ModelCategories)

  const visibleCategories = filteredCategories.filter(
    ([, models]) => models.length > 0,
  )

  const fallbackModel = getFirstAllowedModel(
    visibleCategories.flatMap(([, models]) => models),
    allowedModels,
  )

  const displayModel = selectedModel || fallbackModel || 'Select a model'
  const displayModelAllowed = isModelAllowed(displayModel, allowedModels)

  const triggerLabel = label || 'Model'

  const handleSelect = (model: string) => {
    if (disabled || !isModelAllowed(model, allowedModels)) {
      return
    }

    onModelChange(model)
    setOpen(false)
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button
          size="sm"
          variant={tone === 'ghost' ? 'ghost' : 'outline'}
          className={cn(
            tone === 'ghost'
              ? 'bg-transparent text-current hover:bg-muted'
              : 'border-input bg-background text-foreground hover:bg-muted',
            compact
              ? 'w-auto min-w-[72px] justify-center gap-1.5 px-2 text-sm'
              : 'w-full justify-between gap-2 text-sm',
            active &&
              (tone === 'ghost' ? 'bg-muted font-bold' : 'ring-2 ring-ring'),
            className,
          )}
          disabled={disabled}
        >
          {compact ? (
            <span className="text-sm font-semibold">{triggerLabel}</span>
          ) : (
            <span className="flex min-w-0 flex-col text-left">
              {label && (
                <span className="text-[10px] uppercase tracking-tight text-muted-foreground">
                  {label}
                </span>
              )}
              <span className="flex items-center gap-2 truncate">
                <span
                  className={cn(
                    'truncate',
                    !displayModelAllowed && 'text-muted-foreground',
                  )}
                >
                  {displayModel}
                </span>
                {isFreeModel(displayModel) && (
                  <Badge variant="success" className="text-[10px]">
                    Free
                  </Badge>
                )}
              </span>
            </span>
          )}
          <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-h-[420px] w-[320px] overflow-y-auto"
      >
        {visibleCategories.map(([category, models]) => (
          <div key={category}>
            <div className="px-2 py-1.5 text-xs font-semibold text-muted-foreground">
              {category}
            </div>
            {models.map((model) => {
              const modelAllowed = isModelAllowed(model, allowedModels)
              const itemDisabled = disabled || !modelAllowed

              return (
                <DropdownMenuItem
                  key={model}
                  disabled={itemDisabled}
                  onSelect={(event) => {
                    if (itemDisabled) {
                      event.preventDefault()
                      return
                    }

                    handleSelect(model)
                  }}
                  className={cn(
                    'flex items-center justify-between',
                    !itemDisabled && 'cursor-pointer',
                    selectedModel === model && 'bg-muted',
                    itemDisabled &&
                      'cursor-not-allowed text-muted-foreground opacity-100',
                  )}
                >
                  <span className="truncate">{model}</span>
                  <span
                    className={cn('flex gap-1', itemDisabled && 'opacity-60')}
                  >
                    {isFreeModel(model) && (
                      <Badge variant="success" className="text-[10px]">
                        Free
                      </Badge>
                    )}
                  </span>
                </DropdownMenuItem>
              )
            })}
          </div>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
