'use client'

import { useState } from 'react'
import { CheckCircle2, ChevronRight, XCircle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { FilterBar } from '@/components/platform/filter-bar'
import { SearchInput } from '@/components/platform/search-input'
import { sanitizeAuditEntry } from '@/lib/rbac'
import { cn } from '@/lib/utils'
import type { AuditLogEntry } from '@/lib/types'

interface AuditLogTableProps {
  entries: AuditLogEntry[]
}

export function AuditLogTable({ entries }: AuditLogTableProps) {
  const [query, setQuery] = useState('')
  const [active, setActive] = useState<AuditLogEntry | null>(null)
  const sanitized = entries.map(sanitizeAuditEntry)
  const filtered = sanitized.filter((entry) => {
    const q = query.trim().toLowerCase()
    if (!q) return true
    return (
      entry.actor.toLowerCase().includes(q) ||
      entry.action.toLowerCase().includes(q) ||
      entry.resource.toLowerCase().includes(q) ||
      entry.project.toLowerCase().includes(q) ||
      entry.ip.toLowerCase().includes(q)
    )
  })

  return (
    <>
      <div className="border-b border-border p-4">
        <FilterBar>
          <SearchInput
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search actor, action, resource…"
            className="max-w-sm"
          />
          <span className="text-xs text-muted-foreground">{filtered.length} entries</span>
        </FilterBar>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Timestamp</TableHead>
            <TableHead>Actor</TableHead>
            <TableHead>Action</TableHead>
            <TableHead>Resource</TableHead>
            <TableHead>Project</TableHead>
            <TableHead>IP</TableHead>
            <TableHead>Result</TableHead>
            <TableHead className="w-8" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {filtered.map((entry) => (
            <TableRow
              key={entry.id}
              className="cursor-pointer"
              onClick={() => setActive(entry)}
            >
              <TableCell className="font-mono text-xs text-muted-foreground">
                {entry.timestamp}
              </TableCell>
              <TableCell className="text-sm">{entry.actor}</TableCell>
              <TableCell className="font-mono text-xs">{entry.action}</TableCell>
              <TableCell>
                <Badge variant="outline" className="max-w-[12rem] truncate text-[10px]">
                  {entry.resource}
                </Badge>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">{entry.project}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">{entry.ip}</TableCell>
              <TableCell>
                <span
                  className={cn(
                    'inline-flex items-center gap-1.5 text-xs font-medium',
                    entry.result === 'success' ? 'text-success' : 'text-critical',
                  )}
                >
                  {entry.result === 'success' ? (
                    <CheckCircle2 className="size-3.5" />
                  ) : (
                    <XCircle className="size-3.5" />
                  )}
                  {entry.result}
                </span>
              </TableCell>
              <TableCell>
                <ChevronRight data-icon className="size-4 text-muted-foreground" />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Sheet open={!!active} onOpenChange={(open) => !open && setActive(null)}>
        <SheetContent className="sm:max-w-md">
          {active ? (
            <>
              <SheetHeader>
                <SheetTitle className="font-mono text-sm">{active.action}</SheetTitle>
                <SheetDescription>
                  Audit detail with before/after metadata. Secret values are never displayed.
                </SheetDescription>
              </SheetHeader>
              <div className="flex flex-col gap-4 overflow-y-auto px-4 pb-6">
                <DetailRow label="Timestamp" value={active.timestamp} mono />
                <DetailRow label="Actor" value={active.actor} />
                <DetailRow label="Resource" value={active.resource} mono />
                <DetailRow label="Project" value={active.project} />
                <DetailRow label="IP" value={active.ip} mono />
                <DetailRow label="Result" value={active.result} />

                <MetadataBlock title="Before" data={active.before} />
                <MetadataBlock title="After" data={active.after} />
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </>
  )
}

function DetailRow({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('text-right text-sm text-foreground', mono && 'font-mono text-xs')}>
        {value}
      </span>
    </div>
  )
}

function MetadataBlock({
  title,
  data,
}: {
  title: string
  data?: Record<string, string>
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border bg-secondary/40 p-3">
      <span className="text-xs font-medium text-muted-foreground">{title}</span>
      {data && Object.keys(data).length > 0 ? (
        Object.entries(data).map(([key, value]) => (
          <div key={key} className="flex justify-between gap-2 font-mono text-xs">
            <span className="text-muted-foreground">{key}</span>
            <span className="text-foreground">{value}</span>
          </div>
        ))
      ) : (
        <span className="text-xs text-muted-foreground">No metadata</span>
      )}
    </div>
  )
}
