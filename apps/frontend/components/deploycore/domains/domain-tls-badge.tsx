import { Check, Loader2, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  buildDomainLifecycleSteps,
  DOMAIN_TLS_LABELS,
  tlsToneClasses,
} from '@/lib/domains'
import type { DomainTlsState } from '@/lib/types'

interface DomainTlsBadgeProps {
  state: DomainTlsState
  className?: string
  showDot?: boolean
}

export function DomainTlsBadge({ state, className, showDot = true }: DomainTlsBadgeProps) {
  const tone = tlsToneClasses(state)
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium',
        tone.bg,
        tone.text,
        tone.border,
        className,
      )}
    >
      {showDot ? (
        <span
          className={cn(
            'size-1.5 shrink-0 rounded-full',
            tone.dot,
            (state === 'VERIFYING' || state === 'ISSUING') && 'animate-pulse',
          )}
        />
      ) : null}
      {DOMAIN_TLS_LABELS[state]}
    </span>
  )
}

interface DomainLifecycleProps {
  state: DomainTlsState
  className?: string
}

export function DomainLifecycle({ state, className }: DomainLifecycleProps) {
  const steps = buildDomainLifecycleSteps(state)

  return (
    <ol className={cn('flex flex-col', className)}>
      {steps.map((step, index) => {
        const isLast = index === steps.length - 1
        return (
          <li key={`${step.phase}-${index}`} className="flex gap-3">
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full border text-[10px]',
                  step.status === 'complete' && 'border-success bg-success/10 text-success',
                  step.status === 'active' && 'border-primary bg-primary/10 text-primary',
                  step.status === 'failed' && 'border-critical bg-critical/10 text-critical',
                  step.status === 'pending' && 'border-border bg-muted text-muted-foreground',
                )}
              >
                {step.status === 'complete' && <Check className="size-3" />}
                {step.status === 'active' && <Loader2 className="size-3 animate-spin" />}
                {step.status === 'failed' && <X className="size-3" />}
              </div>
              {!isLast ? (
                <div
                  className={cn(
                    'w-px flex-1',
                    step.status === 'complete' ? 'bg-success/40' : 'bg-border',
                    step.status === 'failed' && 'bg-critical/30',
                  )}
                  style={{ minHeight: '1.25rem' }}
                />
              ) : null}
            </div>
            <div
              className={cn(
                'pb-4 text-sm',
                step.status === 'pending' ? 'text-muted-foreground' : 'text-foreground',
              )}
            >
              <div className="font-medium">{step.label}</div>
              <div className="mt-0.5 font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
                {step.phase}
              </div>
            </div>
          </li>
        )
      })}
    </ol>
  )
}
