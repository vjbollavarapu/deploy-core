import { cn } from '@/lib/utils'
import { STATUS_CONFIG, TONE_CLASSES } from '@/lib/status'
import type { Status } from '@/lib/types'

interface HealthIndicatorProps {
  status: Status
  label?: string
  className?: string
}

export function HealthIndicator({ status, label, className }: HealthIndicatorProps) {
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.unknown
  const tone = TONE_CLASSES[config.tone]

  return (
    <span className={cn('inline-flex items-center gap-2 text-sm', className)}>
      <span className="relative flex size-2">
        <span className={cn('absolute inset-0 rounded-full', tone.dot)} />
        {status === 'deploying' && (
          <span className={cn('absolute inset-0 animate-ping rounded-full opacity-60', tone.dot)} />
        )}
      </span>
      <span className="text-foreground">{label ?? config.label}</span>
    </span>
  )
}
