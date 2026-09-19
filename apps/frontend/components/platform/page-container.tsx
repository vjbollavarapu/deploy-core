import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface PageContainerProps {
  children: ReactNode
  className?: string
  /** Wider max width for dense tables / ops views. Default is constrained. */
  density?: 'default' | 'wide' | 'full'
}

const densityClass: Record<NonNullable<PageContainerProps['density']>, string> = {
  default: 'max-w-7xl',
  wide: 'max-w-[90rem]',
  full: 'max-w-none',
}

export function PageContainer({ children, className, density = 'default' }: PageContainerProps) {
  return (
    <div className={cn('mx-auto flex w-full flex-col gap-6', densityClass[density], className)}>
      {children}
    </div>
  )
}
