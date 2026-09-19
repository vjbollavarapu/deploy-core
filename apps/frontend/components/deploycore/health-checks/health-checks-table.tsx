import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { cn } from '@/lib/utils'
import type { HealthCheckConfig } from '@/lib/types'

interface HealthChecksTableProps {
  checks: HealthCheckConfig[]
}

export function HealthChecksTable({ checks }: HealthChecksTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Application</TableHead>
          <TableHead>Check</TableHead>
          <TableHead>Interval</TableHead>
          <TableHead>Latency</TableHead>
          <TableHead>Recent history</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {checks.map((check) => {
          const lastLatency = check.history.at(-1)?.latencyMs ?? 0
          return (
            <TableRow key={check.id}>
              <TableCell className="font-medium text-foreground">{check.application}</TableCell>
              <TableCell>
                <div className="flex items-center gap-1.5">
                  <Badge variant="secondary" className="text-[10px]">
                    {check.type}
                  </Badge>
                  <span className="font-mono text-xs text-muted-foreground">
                    {check.path}
                    {check.port > 0 ? `:${check.port}` : ''}
                  </span>
                </div>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">{check.interval}s</TableCell>
              <TableCell className={cn('font-mono text-sm', lastLatency > 150 ? 'text-warning' : 'text-foreground')}>
                {lastLatency}ms
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-0.5">
                  {check.history.slice(-12).map((h, i) => (
                    <span
                      key={i}
                      className={cn(
                        'h-3.5 w-1.5 rounded-full',
                        h.status === 'pass' ? 'bg-success/70' : 'bg-critical/70',
                      )}
                    />
                  ))}
                </div>
              </TableCell>
              <TableCell>
                <StatusBadge status={check.status} showDot />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
