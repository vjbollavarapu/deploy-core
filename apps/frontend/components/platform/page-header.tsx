import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface PageHeaderProps {
  title: string
  description?: string
  actions?: ReactNode
  className?: string
  eyebrow?: string
  breadcrumbs?: ReactNode
}

export function PageHeader({
  title,
  description,
  actions,
  className,
  eyebrow,
  breadcrumbs,
}: PageHeaderProps) {
  return (
    <div className={cn('flex flex-col gap-3', className)}>
      {breadcrumbs}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 flex-col gap-1">
          {eyebrow && (
            <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              {eyebrow}
            </span>
          )}
          <h1 className="text-balance text-xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description && <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  )
}
