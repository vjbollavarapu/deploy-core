import type { ReactNode } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

interface DataTableProps {
  children: ReactNode
  className?: string
  toolbar?: ReactNode
  footer?: ReactNode
}

/** Standard bordered table shell for operations console density. */
export function DataTable({ children, className, toolbar, footer }: DataTableProps) {
  return (
    <div className={cn('overflow-hidden rounded-lg border border-border bg-card', className)}>
      {toolbar && <div className="border-b border-border px-3 py-2">{toolbar}</div>}
      {children}
      {footer && <div className="border-t border-border px-3 py-2">{footer}</div>}
    </div>
  )
}

export { Table, TableBody, TableCell, TableHead, TableHeader, TableRow }
