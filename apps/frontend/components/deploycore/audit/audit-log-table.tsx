'use client'

import { useState } from 'react'
import { CheckCircle2, ChevronRight, Lock, ShieldCheck, XCircle } from 'lucide-react'
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
      entry.ip.toLowerCase().includes(q) ||
      entry.result.toLowerCase().includes(q)
    )
  })

  return (
    <>
      <div className="border-b border-border p-4">
        <FilterBar>
          <SearchInput
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search timestamp, actor, action, resource, IP…"
            className="max-w-sm"
          />
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="gap-1 text-[11px] text-muted-foreground">
              <Lock className="size-3" />
              Secrets Never Displayed
            </Badge>
            <span className="text-xs text-muted-foreground">{filtered.length} entries</span>
          </div>
        </FilterBar>
      </div>

      {filtered.length === 0 ? (
        <div className="p-12 text-center text-sm text-muted-foreground">
          No audit log entries matching your search criteria.
        </div>
      ) : (
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
                className="cursor-pointer hover:bg-muted/50"
                onClick={() => setActive(entry)}
              >
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {entry.timestamp}
                </TableCell>
                <TableCell className="text-sm font-medium text-foreground">{entry.actor}</TableCell>
                <TableCell className="font-mono text-xs text-foreground">{entry.action}</TableCell>
                <TableCell>
                  <Badge variant="outline" className="max-w-[14rem] truncate text-[10px]">
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
                    <span className="capitalize">{entry.result}</span>
                  </span>
                </TableCell>
                <TableCell>
                  <ChevronRight data-icon className="size-4 text-muted-foreground" />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Sheet open={!!active} onOpenChange={(open) => !open && setActive(null)}>
        <SheetContent className="overflow-y-auto sm:max-w-lg">
          {active ? (
            <>
              <SheetHeader className="border-b border-border pb-4">
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="font-mono text-[10px]">
                    Audit ID: {active.id.slice(0, 8)}
                  </Badge>
                  <Badge variant="outline" className="gap-1 text-[10px] text-success">
                    <ShieldCheck className="size-3" />
                    Immutable
                  </Badge>
                </div>
                <SheetTitle className="pt-1 font-mono text-base font-semibold">
                  {active.action}
                </SheetTitle>
                <SheetDescription className="text-xs">
                  Event details with before and after state metadata. Secret and credential values are sealed and never rendered.
                </SheetDescription>
              </SheetHeader>

              <div className="flex flex-col gap-4 py-4">
                <div className="flex flex-col gap-2 rounded-md border border-border p-3">
                  <DetailRow label="Timestamp" value={active.timestamp} mono />
                  <DetailRow label="Actor" value={active.actor} />
                  <DetailRow label="Action" value={active.action} mono />
                  <DetailRow label="Resource" value={active.resource} mono />
                  <DetailRow label="Project" value={active.project} />
                  <DetailRow label="Client IP" value={active.ip} mono />
                  <DetailRow
                    label="Result"
                    value={active.result.toUpperCase()}
                    highlight={active.result === 'success' ? 'success' : 'critical'}
                  />
                </div>

                <MetadataBlock
                  title="Before State (Previous Metadata)"
                  data={active.before}
                  comparisonData={active.after}
                />

                <MetadataBlock
                  title="After State (Updated Metadata)"
                  data={active.after}
                  comparisonData={active.before}
                />
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
  highlight,
}: {
  label: string
  value: string
  mono?: boolean
  highlight?: 'success' | 'critical'
}) {
  return (
    <div className="flex items-start justify-between gap-3 py-1">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span
        className={cn(
          'text-right text-xs text-foreground',
          mono && 'font-mono',
          highlight === 'success' && 'font-semibold text-success',
          highlight === 'critical' && 'font-semibold text-critical',
        )}
      >
        {value}
      </span>
    </div>
  )
}

function MetadataBlock({
  title,
  data,
  comparisonData,
}: {
  title: string
  data?: Record<string, string>
  comparisonData?: Record<string, string>
}) {
  const hasData = data && Object.keys(data).length > 0

  return (
    <div className="flex flex-col gap-2 rounded-md border border-border bg-secondary/30 p-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-foreground">{title}</span>
        {hasData && (
          <span className="text-[10px] text-muted-foreground">
            {Object.keys(data).length} key{Object.keys(data).length === 1 ? '' : 's'}
          </span>
        )}
      </div>

      {hasData ? (
        <div className="flex flex-col gap-1.5 divide-y divide-border/40 pt-1">
          {Object.entries(data).map(([key, value]) => {
            const isChanged = comparisonData && comparisonData[key] !== value
            const isMasked = value === '••••••••'

            return (
              <div key={key} className="flex items-start justify-between gap-2 pt-1.5 font-mono text-xs">
                <span className="text-muted-foreground">{key}</span>
                <span
                  className={cn(
                    'break-all text-right text-foreground',
                    isMasked && 'text-amber-500/90 font-bold',
                    isChanged && !isMasked && 'text-info font-medium',
                  )}
                >
                  {value}
                </span>
              </div>
            )
          })}
        </div>
      ) : (
        <span className="pt-1 text-xs text-muted-foreground italic">No state recorded</span>
      )}
    </div>
  )
}
