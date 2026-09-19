import { cn } from '@/lib/utils'

interface ProviderBadgeProps {
  provider: string
  className?: string
}

export function ProviderBadge({ provider, className }: ProviderBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground',
        className,
      )}
    >
      {provider}
    </span>
  )
}
