import { Check, Loader2, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { DEPLOYMENT_FAILURE_LABELS } from '@/lib/deployments'
import type { DeploymentStep } from '@/lib/types'

interface DeploymentPipelineProps {
  steps: DeploymentStep[]
  className?: string
}

export function DeploymentPipeline({ steps, className }: DeploymentPipelineProps) {
  return (
    <ol className={cn('flex flex-col gap-0', className)}>
      {steps.map((step, i) => {
        const isLast = i === steps.length - 1
        return (
          <li key={step.phase} className="flex gap-3">
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full border text-[10px] font-medium',
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
              {!isLast && (
                <div
                  className={cn(
                    'w-px flex-1',
                    step.status === 'complete' ? 'bg-success/40' : 'bg-border',
                    step.status === 'failed' && 'bg-critical/30',
                  )}
                  style={{ minHeight: '1.25rem' }}
                />
              )}
            </div>
            <div
              className={cn(
                'pb-5 text-sm',
                step.status === 'pending' ? 'text-muted-foreground' : 'text-foreground',
              )}
            >
              <div className="font-medium">{step.name}</div>
              <div className="mt-0.5 font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
                {step.phase}
              </div>
              {step.failureReason ? (
                <div className="mt-1 text-xs text-critical">
                  {DEPLOYMENT_FAILURE_LABELS[step.failureReason]}
                </div>
              ) : null}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
