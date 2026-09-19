'use client'

import type { ComponentProps } from 'react'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

interface SearchInputProps extends Omit<ComponentProps<'input'>, 'type'> {
  containerClassName?: string
}

export function SearchInput({ className, containerClassName, ...props }: SearchInputProps) {
  return (
    <div className={cn('relative', containerClassName)}>
      <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
      <Input type="search" className={cn('h-8 pl-8', className)} {...props} />
    </div>
  )
}
