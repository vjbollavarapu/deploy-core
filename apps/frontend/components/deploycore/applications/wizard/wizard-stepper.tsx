'use client'

import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import { WIZARD_STEPS } from '@/lib/validations/application'

interface WizardStepperProps {
  step: number
  onStepClick?: (stepIndex: number) => void
}

export function WizardStepper({ step, onStepClick }: WizardStepperProps) {
  return (
    <ol className="grid grid-cols-4 gap-2 sm:grid-cols-8" aria-label="Wizard progress">
      {WIZARD_STEPS.map((item, index) => {
        const complete = index < step
        const current = index === step
        const isClickable = complete && Boolean(onStepClick)

        return (
          <li key={item.id} className="flex min-w-0 flex-col items-center gap-1">
            <button
              type="button"
              disabled={!isClickable}
              onClick={() => isClickable && onStepClick?.(index)}
              className={cn(
                'flex size-6 items-center justify-center rounded-full text-[11px] font-medium transition-colors',
                complete && 'bg-primary text-primary-foreground hover:bg-primary/90 cursor-pointer',
                current && 'bg-primary/15 text-primary ring-1 ring-primary/40 cursor-default',
                !complete && !current && 'bg-muted text-muted-foreground cursor-not-allowed',
              )}
              aria-current={current ? 'step' : undefined}
              aria-label={`Step ${index + 1}: ${item.label}${complete ? ' (completed, click to edit)' : ''}`}
            >
              {complete ? <Check className="size-3.5" aria-hidden /> : index + 1}
            </button>
            <span
              className={cn(
                'truncate text-center text-[10px] leading-tight',
                current ? 'font-medium text-foreground' : 'text-muted-foreground',
              )}
            >
              {item.label}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
