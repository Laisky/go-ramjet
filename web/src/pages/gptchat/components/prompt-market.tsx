import { Search, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { MarketPrompts } from '../data/market-prompts'

interface PromptMarketProps {
  onAddPrompt: (name: string, prompt: string) => void
}

/**
 * PromptMarket allows users to browse and add predefined prompts.
 */
export function PromptMarket({ onAddPrompt }: PromptMarketProps) {
  const [search, setSearch] = useState('')

  const filteredPrompts = useMemo(() => {
    const term = search.toLowerCase().trim()
    if (!term) return MarketPrompts
    return MarketPrompts.filter(
      (p) =>
        p.name.toLowerCase().includes(term) ||
        p.prompt.toLowerCase().includes(term),
    )
  }, [search])

  return (
    <div className="space-y-4">
      <div className="relative">
        <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search prompts..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-8"
        />
      </div>

      <div className="h-[300px] overflow-y-auto pr-4">
        <div className="grid grid-cols-1 gap-2">
          {filteredPrompts.map((p, i) => (
            <div
              key={i}
              className="group flex flex-col gap-1 rounded-lg border border-border p-3 transition-colors hover:bg-muted/50"
            >
              <div className="flex items-center justify-between">
                <span className="text-sm font-semibold">{p.name}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-8 w-8 p-0 opacity-0 group-hover:opacity-100"
                  onClick={() => onAddPrompt(p.name, p.prompt)}
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
              <p className="line-clamp-2 text-xs text-muted-foreground">
                {p.prompt}
              </p>
              <div className="mt-1 flex flex-wrap gap-1">
                {/* You could add tags here if they were in the data */}
                <Badge variant="outline" className="text-[10px] px-1 py-0">
                  Market
                </Badge>
              </div>
            </div>
          ))}
          {filteredPrompts.length === 0 && (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No prompts found matching "{search}"
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
