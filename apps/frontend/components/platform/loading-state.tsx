import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

interface LoadingStateProps {
  variant?: 'page' | 'section' | 'inline' | 'table'
  rows?: number
  className?: string
  label?: string
}

export function LoadingState({
  variant = 'section',
  rows = 5,
  className,
  label = 'Loading…',
}: LoadingStateProps) {
  if (variant === 'inline') {
    return (
      <div
        className={cn('inline-flex items-center gap-2 text-sm text-muted-foreground', className)}
        role="status"
        aria-live="polite"
      >
        <Spinner className="size-3.5" />
        <span>{label}</span>
      </div>
    )
  }

  if (variant === 'table') {
    return (
      <div className={cn('flex flex-col gap-2 p-3', className)} role="status" aria-label={label}>
        {Array.from({ length: rows }).map((_, i) => (
          <Skeleton key={i} className="h-9 w-full" />
        ))}
      </div>
    )
  }

  if (variant === 'page') {
    return (
      <div className={cn('flex flex-col gap-6', className)} role="status" aria-label={label}>
        <div className="flex flex-col gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-7 w-56" />
          <Skeleton className="h-4 w-80 max-w-full" />
        </div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  return (
    <div
      className={cn('flex min-h-32 flex-col items-center justify-center gap-3 py-8', className)}
      role="status"
      aria-live="polite"
    >
      <Spinner className="size-5" />
      <span className="text-sm text-muted-foreground">{label}</span>
    </div>
  )
}
