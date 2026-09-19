'use client'

import Link from 'next/link'
import { DownloadCloud } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState } from '@/components/platform/empty-state'
import { StatusBadge } from '@/components/platform/status-badge'
import { getDatabaseBackupJob, getDatabaseBackupRuns } from '@/lib/databases'
import type { BackupRun, DatabaseInstance } from '@/lib/types'

interface DatabaseBackupsPanelProps {
  database: DatabaseInstance
}

export function DatabaseBackupsPanel({ database }: DatabaseBackupsPanelProps) {
  const job = getDatabaseBackupJob(database)
  const runs = getDatabaseBackupRuns(database)

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
          <div className="flex flex-col gap-1">
            <CardTitle>Backup policy</CardTitle>
            <CardDescription>
              Schedule, destination, and retention for {database.name}.
            </CardDescription>
          </div>
          <Button size="sm" variant="outline">
            <DownloadCloud data-icon="inline-start" />
            Backup now
          </Button>
        </CardHeader>
        <CardContent>
          {job ? (
            <div className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <PolicyField label="Policy" value={job.policy} />
              <PolicyField label="Destination" value={job.destination} mono />
              <PolicyField label="Retention" value={job.retention} />
              <PolicyField label="Last success" value={job.lastSuccess} />
              <PolicyField label="Next run" value={job.nextRun} />
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Status</span>
                <StatusBadge status={job.status} showDot />
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No backup job is configured.</p>
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Backup runs</CardTitle>
          <CardDescription>
            Status, timing, size, destination, and checksum for each run.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {runs.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={DownloadCloud}
                title="No backup runs"
                description="Completed and in-progress backup runs will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <BackupRunsTable runs={runs} />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export function BackupRunsTable({
  runs,
  showDatabase,
}: {
  runs: BackupRun[]
  showDatabase?: boolean
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {showDatabase ? <TableHead>Database</TableHead> : null}
          <TableHead>Status</TableHead>
          <TableHead>Started</TableHead>
          <TableHead>Completed</TableHead>
          <TableHead>Duration</TableHead>
          <TableHead>Size</TableHead>
          <TableHead>Destination</TableHead>
          <TableHead>Checksum</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {runs.map((run) => (
          <TableRow key={run.id}>
            {showDatabase ? (
              <TableCell>
                <Link
                  href={`/databases/${run.databaseId}/backups`}
                  className="font-medium hover:underline"
                >
                  {run.database}
                </Link>
              </TableCell>
            ) : null}
            <TableCell>
              <Badge
                variant={run.status === 'success' ? 'secondary' : 'outline'}
                className={
                  run.status === 'failed'
                    ? 'text-critical'
                    : run.status === 'running'
                      ? 'text-warning'
                      : undefined
                }
              >
                {run.status}
              </Badge>
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{run.startedAt}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{run.completedAt}</TableCell>
            <TableCell className="tabular text-muted-foreground">{run.duration}</TableCell>
            <TableCell className="tabular text-muted-foreground">
              {run.sizeGb > 0 ? `${run.sizeGb} GB` : '—'}
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{run.destination}</TableCell>
            <TableCell className="max-w-[12rem] truncate font-mono text-[11px] text-muted-foreground">
              {run.checksum}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function PolicyField({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={mono ? 'font-mono text-xs text-foreground' : 'text-foreground'}>{value}</span>
    </div>
  )
}
