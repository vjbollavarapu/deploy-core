'use client'

import { useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FilterBar } from '@/components/platform/filter-bar'
import { getPlatformEvents } from '@/lib/observability'
import { TONE_CLASSES } from '@/lib/status'
import { cn } from '@/lib/utils'
import type { PlatformEvent } from '@/lib/types'

const CATEGORIES = [
  'all',
  'deployment',
  'infrastructure',
  'security',
  'backup',
  'certificate',
  'incident',
] as const

export function EventsFeed() {
  const [category, setCategory] = useState<(typeof CATEGORIES)[number]>('all')
  const [query, setQuery] = useState('')
  const events = useMemo(() => getPlatformEvents(), [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return events.filter((event) => {
      if (category !== 'all' && event.category !== category) return false
      if (!q) return true
      return (
        event.actor.toLowerCase().includes(q) ||
        event.action.toLowerCase().includes(q) ||
        event.target.toLowerCase().includes(q) ||
        (event.project?.toLowerCase().includes(q) ?? false) ||
        (event.environment?.toLowerCase().includes(q) ?? false)
      )
    })
  }, [category, events, query])

  return (
    <div className="flex flex-col gap-4">
      <FilterBar>
        <Select
          value={category}
          onValueChange={(v) => setCategory((v as (typeof CATEGORIES)[number]) ?? 'all')}
        >
          <SelectTrigger className="w-48" aria-label="Filter by category">
            <SelectValue placeholder="Category" />
          </SelectTrigger>
          <SelectContent>
            {CATEGORIES.map((item) => (
              <SelectItem key={item} value={item}>
                {item === 'all' ? 'All categories' : item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <InputGroup className="max-w-xs">
          <InputGroupInput
            placeholder="Search events…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <InputGroupAddon>
            <Search className="size-4 text-muted-foreground" />
          </InputGroupAddon>
        </InputGroup>

        <span className="text-xs text-muted-foreground">{filtered.length} events</span>
      </FilterBar>

      <Card size="sm">
        <CardContent className="p-0">
          <ul className="flex flex-col">
            {filtered.map((event) => (
              <EventRow key={event.id} event={event} />
            ))}
            {filtered.length === 0 ? (
              <li className="p-6 text-center text-sm text-muted-foreground">
                No events match this filter.
              </li>
            ) : null}
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}

function EventRow({ event }: { event: PlatformEvent }) {
  const tone = TONE_CLASSES[event.tone]
  return (
    <li className="flex gap-3 border-b border-border/60 px-4 py-3 last:border-0">
      <span className={cn('mt-1.5 size-1.5 shrink-0 rounded-full', tone.dot)} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary" className="text-[10px] capitalize">
            {event.category}
          </Badge>
          {event.environment ? (
            <span className="text-[11px] text-muted-foreground">{event.environment}</span>
          ) : null}
          {event.project ? (
            <span className="text-[11px] text-muted-foreground">{event.project}</span>
          ) : null}
        </div>
        <p className="text-pretty text-sm text-foreground">
          <span className="font-medium">{event.actor}</span>{' '}
          <span className="text-muted-foreground">{event.action}</span>{' '}
          <span className="font-medium">{event.target}</span>
        </p>
        <span className="text-xs text-muted-foreground">{event.timestamp}</span>
      </div>
    </li>
  )
}
