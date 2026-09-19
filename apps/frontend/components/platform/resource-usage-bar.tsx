import { cn } from '@/lib/utils'

interface ResourceUsageBarProps {
  label?: string
  value: number
  detail?: string
  className?: string
  size?: 'sm' | 'md'
}

function toneForValue(value: number) {
  if (value >= 90) return 'bg-critical'
  if (value >= 75) return 'bg-warning'
  return 'bg-primary'
}

export function ResourceUsageBar({ label, value, detail, className, size = 'md' }: ResourceUsageBarProps) {
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      {(label || detail) && (
        <div className="flex items-center justify-between gap-2 text-xs">
          {label && <span className="text-muted-foreground">{label}</span>}
          {detail && <span className="tabular text-foreground">{detail}</span>}
        </div>
      )}
      <div className={cn('w-full overflow-hidden rounded-full bg-muted', size === 'sm' ? 'h-1.5' : 'h-2')}>
        <div
          className={cn('h-full rounded-full transition-all', toneForValue(value))}
          style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
        />
      </div>
    </div>
  )
}
