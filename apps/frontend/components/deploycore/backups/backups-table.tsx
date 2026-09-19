'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ChevronRight, DownloadCloud } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import { BackupRunsTable } from '@/components/deploycore/databases/database-backups-panel'
import type { BackupJob, BackupRun } from '@/lib/types'

interface BackupsTableProps {
  jobs: BackupJob[]
  runs: BackupRun[]
}

export function BackupsTable({ jobs, runs }: BackupsTableProps) {
  const [active, setActive] = useState<BackupJob | null>(null)
  const activeRuns = active ? runs.filter((r) => r.backupJobId === active.id) : []

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Database</TableHead>
            <TableHead>Policy</TableHead>
            <TableHead>Destination</TableHead>
            <TableHead>Last success</TableHead>
            <TableHead>Next run</TableHead>
            <TableHead>Retention</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-8" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {jobs.map((job) => (
            <TableRow key={job.id} className="cursor-pointer" onClick={() => setActive(job)}>
              <TableCell>
                <Link
                  href={`/databases/${job.databaseId}/backups`}
                  className="font-medium text-foreground hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {job.database}
                </Link>
              </TableCell>
              <TableCell>
                <Badge variant="secondary" className="text-[10px]">
                  {job.policy}
                </Badge>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">{job.destination}</TableCell>
              <TableCell className="text-sm text-muted-foreground">{job.lastSuccess}</TableCell>
              <TableCell className="text-sm text-muted-foreground">{job.nextRun}</TableCell>
              <TableCell className="text-sm text-muted-foreground">{job.retention}</TableCell>
              <TableCell>
                <StatusBadge status={job.status} showDot />
              </TableCell>
              <TableCell>
                <ChevronRight data-icon className="size-4 text-muted-foreground" />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Sheet open={!!active} onOpenChange={(open) => !open && setActive(null)}>
        <SheetContent className="sm:max-w-lg">
          {active && (
            <>
              <SheetHeader>
                <SheetTitle>{active.database}</SheetTitle>
                <SheetDescription>
                  {active.policy} · {active.destination}
                </SheetDescription>
              </SheetHeader>
              <div className="flex flex-col gap-3 px-4 pb-6">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Recent runs
                  </span>
                  <Button
                    size="sm"
                    variant="outline"
                    nativeButton={false}
                    render={<Link href={`/databases/${active.databaseId}/backups`} />}
                  >
                    <DownloadCloud data-icon="inline-start" />
                    Open
                  </Button>
                </div>
                {activeRuns.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No runs recorded.</p>
                ) : (
                  <div className="overflow-x-auto rounded-md border border-border">
                    <BackupRunsTable runs={activeRuns} />
                  </div>
                )}
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  )
}
