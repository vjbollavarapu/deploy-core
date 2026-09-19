import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface FilterBarProps {
  children: ReactNode
  className?: string
  end?: ReactNode
}

export function FilterBar({ children, className, end }: FilterBarProps) {
  return (
    <div
      className={cn(
        'flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between',
        className,
      )}
    >
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{children}</div>
      {end && <div className="flex shrink-0 flex-wrap items-center gap-2">{end}</div>}
    </div>
  )
}
