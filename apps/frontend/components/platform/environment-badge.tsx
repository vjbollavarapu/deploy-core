import { cn } from '@/lib/utils'

interface EnvironmentBadgeProps {
  environment: string
  className?: string
}

const ENV_CLASSES: Record<string, string> = {
  Production: 'border-accent bg-accent text-accent-foreground',
  Staging: 'border-warning/20 bg-warning/10 text-warning',
  Preview: 'border-border bg-muted text-muted-foreground',
  Development: 'border-info/20 bg-info/10 text-info',
}

export function EnvironmentBadge({ environment, className }: EnvironmentBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium',
        ENV_CLASSES[environment] ?? 'border-border bg-muted text-muted-foreground',
        className,
      )}
    >
      {environment}
    </span>
  )
}
