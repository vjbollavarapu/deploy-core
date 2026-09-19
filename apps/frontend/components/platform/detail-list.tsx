import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface DetailListItem {
  label: string
  value: ReactNode
}

interface DetailListProps {
  items: DetailListItem[]
  className?: string
  columns?: 1 | 2
}

export function DetailList({ items, className, columns = 1 }: DetailListProps) {
  return (
    <dl
      className={cn(
        'grid gap-x-6 gap-y-3',
        columns === 2 ? 'sm:grid-cols-2' : 'grid-cols-1',
        className,
      )}
    >
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <dt className="text-xs text-muted-foreground">{item.label}</dt>
          <dd className="mt-0.5 truncate text-sm text-foreground">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}
