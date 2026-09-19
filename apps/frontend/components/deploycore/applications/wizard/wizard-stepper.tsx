'use client'

import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import { WIZARD_STEPS } from '@/lib/validations/application'

interface WizardStepperProps {
  step: number
}

export function WizardStepper({ step }: WizardStepperProps) {
  return (
    <ol className="grid grid-cols-4 gap-2 sm:grid-cols-8" aria-label="Wizard progress">
      {WIZARD_STEPS.map((item, index) => {
        const complete = index < step
        const current = index === step
        return (
          <li key={item.id} className="flex min-w-0 flex-col items-center gap-1">
            <span
              className={cn(
                'flex size-6 items-center justify-center rounded-full text-[11px] font-medium',
                complete && 'bg-primary text-primary-foreground',
                current && 'bg-primary/15 text-primary ring-1 ring-primary/40',
                !complete && !current && 'bg-muted text-muted-foreground',
              )}
              aria-current={current ? 'step' : undefined}
            >
              {complete ? <Check className="size-3.5" aria-hidden /> : index + 1}
            </span>
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
