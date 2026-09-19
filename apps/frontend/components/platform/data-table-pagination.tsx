'use client'

import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface DataTablePaginationProps {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
  className?: string
}

export function DataTablePagination({
  page,
  pageSize,
  total,
  onPageChange,
  className,
}: DataTablePaginationProps) {
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1
  const to = Math.min(page * pageSize, total)

  return (
    <div
      className={cn(
        'flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between',
        className,
      )}
    >
      <p className="text-xs text-muted-foreground tabular">
        {total === 0 ? 'No results' : `${from}–${to} of ${total}`}
      </p>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
          aria-label="Previous page"
        >
          <ChevronLeft className="size-3.5" />
          Prev
        </Button>
        <span className="text-xs text-muted-foreground tabular">
          {page} / {pageCount}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7"
          disabled={page >= pageCount}
          onClick={() => onPageChange(page + 1)}
          aria-label="Next page"
        >
          Next
          <ChevronRight className="size-3.5" />
        </Button>
      </div>
    </div>
  )
}
