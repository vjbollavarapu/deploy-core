import { cn } from '@/lib/utils'
import type { LucideIcon } from 'lucide-react'
import { ArrowDown, ArrowUp } from 'lucide-react'

interface MetricCardProps {
  label: string
  value: string
  detail?: string
  icon?: LucideIcon
  trend?: { value: string; direction: 'up' | 'down'; tone?: 'success' | 'critical' | 'neutral' }
  tone?: 'default' | 'success' | 'warning' | 'critical'
  className?: string
}

const toneRing: Record<string, string> = {
  default: '',
  success: 'ring-success/20',
  warning: 'ring-warning/20',
  critical: 'ring-critical/20',
}

export function MetricCard({
  label,
  value,
  detail,
  icon: Icon,
  trend,
  tone = 'default',
  className,
}: MetricCardProps) {
  return (
    <div
      className={cn(
        'flex flex-col gap-1.5 rounded-lg border border-border bg-card px-3 py-2.5',
        tone !== 'default' && 'ring-1',
        toneRing[tone],
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        {Icon && <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />}
      </div>
      <div className="flex items-baseline gap-2">
        <span className="tabular text-xl font-semibold tracking-tight text-foreground sm:text-2xl">
          {value}
        </span>
        {trend && (
          <span
            className={cn(
              'flex items-center gap-0.5 text-xs font-medium',
              trend.tone === 'success' && 'text-success',
              trend.tone === 'critical' && 'text-critical',
              (!trend.tone || trend.tone === 'neutral') && 'text-muted-foreground',
            )}
          >
            {trend.direction === 'up' ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" />}
            {trend.value}
          </span>
        )}
      </div>
      {detail && <span className="text-xs text-muted-foreground">{detail}</span>}
    </div>
  )
}
