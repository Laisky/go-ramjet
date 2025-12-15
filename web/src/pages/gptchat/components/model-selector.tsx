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
import { ModelCategories, isFreeModel, isImageModel } from '../models'

export interface ModelSelectorProps {
  selectedModel: string
  onModelChange: (model: string) => void
  disabled?: boolean
  label?: string
  categories?: string[]
  active?: boolean
  className?: string
}

/**
 * ModelSelector provides a categorized model selection dropdown.
 */
export function ModelSelector({
  selectedModel,
  onModelChange,
  disabled,
  label,
  categories,
  active,
  className,
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)

  const filteredCategories = categories?.length
    ? Object.entries(ModelCategories).filter(([category]) =>
        categories.includes(category),
      )
    : Object.entries(ModelCategories)

  const displayModel =
    selectedModel || filteredCategories[0]?.[1]?.[0] || 'Select a model'

  const handleSelect = (model: string) => {
    onModelChange(model)
    setOpen(false)
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button
          size="sm"
          variant="outline"
          className={cn(
            'w-full justify-between gap-2 text-sm border-slate-200 bg-white text-slate-900 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:bg-slate-800',
            active && 'ring-2 ring-blue-500/40',
            className,
          )}
          disabled={disabled}
        >
          <span className="flex min-w-0 flex-col text-left">
            {label && (
              <span className="text-[10px] uppercase tracking-tight text-black/60 dark:text-white/60">
                {label}
              </span>
            )}
            <span className="flex items-center gap-2 truncate">
              <span className="truncate">{displayModel}</span>
              {isFreeModel(displayModel) && (
                <Badge variant="success" className="text-[10px]">
                  Free
                </Badge>
              )}
              {isImageModel(displayModel) && (
                <Badge variant="secondary" className="text-[10px]">
                  🎨
                </Badge>
              )}
            </span>
          </span>
          <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-h-[420px] w-[320px] overflow-y-auto"
      >
        {filteredCategories.map(([category, models]) => (
          <div key={category}>
            <div className="px-2 py-1.5 text-xs font-semibold text-black/50 dark:text-white/50">
              {category}
            </div>
            {models.map((model) => (
              <DropdownMenuItem
                key={model}
                onClick={() => handleSelect(model)}
                className={cn(
                  'flex cursor-pointer items-center justify-between',
                  selectedModel === model && 'bg-black/5 dark:bg-white/5',
                )}
              >
                <span className="truncate">{model}</span>
                <span className="flex gap-1">
                  {isFreeModel(model) && (
                    <Badge variant="success" className="text-[10px]">
                      Free
                    </Badge>
                  )}
                  {isImageModel(model) && (
                    <Badge variant="secondary" className="text-[10px]">
                      🎨
                    </Badge>
                  )}
                </span>
              </DropdownMenuItem>
            ))}
          </div>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
