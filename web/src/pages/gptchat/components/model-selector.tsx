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
}

/**
 * ModelSelector provides a categorized model selection dropdown.
 */
export function ModelSelector({
  selectedModel,
  onModelChange,
  disabled,
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)

  const handleSelect = (model: string) => {
    onModelChange(model)
    setOpen(false)
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button
          variant="outline"
          className="w-full justify-between text-sm"
          disabled={disabled}
        >
          <span className="flex items-center gap-2">
            <span className="truncate">{selectedModel}</span>
            {isFreeModel(selectedModel) && (
              <Badge variant="success" className="text-[10px]">
                Free
              </Badge>
            )}
            {isImageModel(selectedModel) && (
              <Badge variant="secondary" className="text-[10px]">
                🎨
              </Badge>
            )}
          </span>
          <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-h-[400px] w-[300px] overflow-y-auto"
      >
        {Object.entries(ModelCategories).map(([category, models]) => (
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
                  selectedModel === model && 'bg-black/5 dark:bg-white/5'
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
