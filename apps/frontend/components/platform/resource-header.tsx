import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { PageHeader } from './page-header'
import { Breadcrumbs, type BreadcrumbCrumb } from './breadcrumbs'

interface ResourceHeaderProps {
  title: string
  description?: string
  actions?: ReactNode
  breadcrumbs?: BreadcrumbCrumb[]
  badges?: ReactNode
  meta?: ReactNode
  className?: string
}

export function ResourceHeader({
  title,
  description,
  actions,
  breadcrumbs,
  badges,
  meta,
  className,
}: ResourceHeaderProps) {
  return (
    <div className={cn('flex flex-col gap-3', className)}>
      <PageHeader
        title={title}
        description={description}
        actions={actions}
        breadcrumbs={breadcrumbs ? <Breadcrumbs items={breadcrumbs} /> : undefined}
      />
      {(badges || meta) && (
        <div className="flex flex-wrap items-center gap-2">
          {badges}
          {meta}
        </div>
      )}
    </div>
  )
}
