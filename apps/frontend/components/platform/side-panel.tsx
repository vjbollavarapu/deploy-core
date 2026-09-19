'use client'

import type { ReactNode } from 'react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

interface SidePanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: ReactNode
  className?: string
  side?: 'right' | 'left'
}

export function SidePanel({
  open,
  onOpenChange,
  title,
  description,
  children,
  className,
  side = 'right',
}: SidePanelProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side={side} className={cn('w-full sm:max-w-md', className)}>
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>
        <div className="mt-4 flex-1 overflow-y-auto px-1">{children}</div>
      </SheetContent>
    </Sheet>
  )
}
