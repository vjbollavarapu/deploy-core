import { cn } from '@/lib/utils'
import { phaseLabel, phaseToneClasses } from '@/lib/deployments'
import type { DeploymentEvent } from '@/lib/types'

interface DeploymentEventTimelineProps {
  events: DeploymentEvent[]
  className?: string
}

export function DeploymentEventTimeline({ events, className }: DeploymentEventTimelineProps) {
  return (
    <ol className={cn('flex flex-col', className)}>
      {events.map((event, index) => {
        const tone = phaseToneClasses(event.tone)
        const isLast = index === events.length - 1
        return (
          <li key={event.id} className="flex gap-3">
            <div className="flex flex-col items-center">
              <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', tone.dot)} />
              {!isLast && <div className="w-px flex-1 bg-border" style={{ minHeight: '1.5rem' }} />}
            </div>
            <div className={cn('min-w-0 flex-1 pb-4', isLast && 'pb-0')}>
              <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
                <span className="font-mono text-[11px] text-muted-foreground">{event.timestamp}</span>
                <span className={cn('text-xs font-medium', tone.text)}>{phaseLabel(event.phase)}</span>
              </div>
              <p className="mt-0.5 text-pretty text-sm text-foreground">{event.message}</p>
            </div>
          </li>
        )
      })}
    </ol>
  )
}
