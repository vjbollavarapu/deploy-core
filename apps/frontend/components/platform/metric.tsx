import { cn } from '@/lib/utils'

interface MetricProps {
  label: string
  value: string
  className?: string
  valueClassName?: string
  align?: 'start' | 'end'
}

export function Metric({ label, value, className, valueClassName, align = 'start' }: MetricProps) {
  return (
    <div className={cn('flex flex-col gap-0.5', align === 'end' && 'items-end text-right', className)}>
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('tabular text-sm font-medium text-foreground', valueClassName)}>{value}</span>
    </div>
  )
}
