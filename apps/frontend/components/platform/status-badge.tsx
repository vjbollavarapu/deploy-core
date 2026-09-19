import { cn } from '@/lib/utils'
import { STATUS_CONFIG, TONE_CLASSES } from '@/lib/status'
import type { Status } from '@/lib/types'

interface StatusBadgeProps {
  status: Status
  className?: string
  showDot?: boolean
}

export function StatusBadge({ status, className, showDot = true }: StatusBadgeProps) {
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.unknown
  const tone = TONE_CLASSES[config.tone]

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
      {showDot && (
        <span
          className={cn(
            'size-1.5 shrink-0 rounded-full',
            tone.dot,
            status === 'deploying' && 'animate-pulse',
          )}
        />
      )}
      {config.label}
    </span>
  )
}
