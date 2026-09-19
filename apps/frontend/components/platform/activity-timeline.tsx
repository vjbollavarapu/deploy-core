import { cn } from '@/lib/utils'
import { TONE_CLASSES } from '@/lib/status'
import type { ActivityItem } from '@/lib/types'

interface ActivityTimelineProps {
  items: ActivityItem[]
  className?: string
}

export function ActivityTimeline({ items, className }: ActivityTimelineProps) {
  return (
    <ul className={cn('flex flex-col', className)}>
      {items.map((item) => {
        const tone = TONE_CLASSES[item.tone]
        return (
          <li key={item.id} className="flex gap-3 border-b border-border/60 py-2.5 last:border-0">
            <span className={cn('mt-1.5 size-1.5 shrink-0 rounded-full', tone.dot)} />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <p className="text-pretty text-sm text-foreground">
                <span className="font-medium">{item.actor}</span>{' '}
                <span className="text-muted-foreground">{item.action}</span>{' '}
                <span className="font-medium">{item.target}</span>
              </p>
              <span className="text-xs text-muted-foreground">{item.timestamp}</span>
            </div>
          </li>
        )
      })}
    </ul>
  )
}

/** @deprecated Prefer ActivityTimeline */
export const ActivityFeed = ActivityTimeline

