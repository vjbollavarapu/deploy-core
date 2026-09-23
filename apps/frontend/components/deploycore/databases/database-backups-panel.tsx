'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { DownloadCloud, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState } from '@/components/platform/empty-state'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient, ApiError } from '@/lib/api'
import {
  getDatabaseBackupJob,
  getDatabaseBackupRuns,
  mapWireBackupToRun,
  type WireBackup,
} from '@/lib/databases'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import type { BackupRun, DatabaseInstance } from '@/lib/types'

interface DatabaseBackupsPanelProps {
  database: DatabaseInstance
}

type BackupPage = {
  items?: WireBackup[]
  totalCount?: number
}

type CreateBackupResponse = {
  backup?: WireBackup
}

export function DatabaseBackupsPanel({ database }: DatabaseBackupsPanelProps) {
  const demo = isDemoModeEnabled()
  const job = getDatabaseBackupJob(database)
  const [runs, setRuns] = useState<BackupRun[]>(() =>
    demo ? getDatabaseBackupRuns(database) : [],
  )
  const [loading, setLoading] = useState(!demo)
  const [backingUp, setBackingUp] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  const reload = useCallback(() => {
    setReloadKey((k) => k + 1)
  }, [])

  useEffect(() => {
    if (demo) {
      return
    }
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setLoadError(null)
      try {
        const page = await apiClient.get<BackupPage>(`/databases/${database.id}/backups?limit=50`)
        if (cancelled) return
        const items = page.items ?? []
        setRuns(items.map((b) => mapWireBackupToRun(b, database)))
      } catch (err) {
        if (cancelled) return
        setRuns([])
        setLoadError(
          err instanceof ApiError ? err.message : 'Could not load backups from the Control Plane.',
        )
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [database, demo, reloadKey])

  async function handleBackupNow() {
    if (demo) {
      toast.message('Demo mode: Backup Now is not submitted to the Control Plane.')
      return
    }

    setBackingUp(true)
    try {
      const created = await apiClient.post<CreateBackupResponse>(
        `/databases/${database.id}/backups`,
        {},
      )
      if (created.backup) {
        setRuns((prev) => {
          const mapped = mapWireBackupToRun(created.backup!, database)
          const without = prev.filter((r) => r.id !== mapped.id)
          return [mapped, ...without]
        })
      }
      reload()
      const status = (created.backup?.status || 'QUEUED').toUpperCase()
      toast.success(`Backup ${status.toLowerCase()} for ${database.name}`)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Failed to trigger database backup')
    } finally {
      setBackingUp(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
          <div className="flex flex-col gap-1">
            <CardTitle>Backup policy</CardTitle>
            <CardDescription>
              Automated snapshot schedule, destination, and retention for {database.name}.
            </CardDescription>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={backingUp || loading}
            onClick={() => void handleBackupNow()}
          >
            {backingUp ? (
              <Loader2 className="animate-spin" data-icon="inline-start" />
            ) : (
              <DownloadCloud data-icon="inline-start" />
            )}
            {backingUp ? 'Backing up…' : 'Backup now'}
          </Button>
        </CardHeader>
        <CardContent>
          {job ? (
            <div className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <PolicyField label="Policy schedule" value={job.policy} />
              <PolicyField label="Destination" value={job.destination} mono />
              <PolicyField label="Retention" value={job.retention} />
              <PolicyField label="Last success" value={job.lastSuccess} />
              <PolicyField label="Next scheduled run" value={job.nextRun} />
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Status</span>
                <StatusBadge status={job.status} showDot />
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {demo
                ? 'No automated backup policy configured.'
                : 'Backup policy metadata is managed by the Control Plane. Use Backup now to queue an ad-hoc snapshot.'}
            </p>
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Backup runs</CardTitle>
          <CardDescription>
            Audited history: status, started, completed, duration, size, destination, and checksum.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Loading backups…
            </div>
          ) : loadError ? (
            <div className="flex flex-col gap-2 p-4">
              <p className="text-sm text-destructive">{loadError}</p>
              <Button size="sm" variant="outline" className="w-fit" onClick={reload}>
                Retry
              </Button>
            </div>
          ) : runs.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={DownloadCloud}
                title="No backup runs"
                description="Completed and in-progress backup runs will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <BackupRunsTable runs={runs} />
            </div>
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
    <div className="overflow-x-auto">
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
                      ? 'text-destructive border-destructive/30'
                      : run.status === 'running' || run.status === 'queued'
                        ? 'text-amber-600 dark:text-amber-400 border-amber-500/30'
                        : undefined
                  }
                >
                  {run.status}
                </Badge>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground whitespace-nowrap">{run.startedAt}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground whitespace-nowrap">{run.completedAt}</TableCell>
              <TableCell className="tabular text-muted-foreground whitespace-nowrap">{run.duration}</TableCell>
              <TableCell className="tabular text-muted-foreground whitespace-nowrap">
                {run.sizeGb > 0 ? `${run.sizeGb} GB` : '—'}
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground max-w-[14rem] truncate" title={run.destination}>
                {run.destination}
              </TableCell>
              <TableCell className="max-w-[12rem] truncate font-mono text-[11px] text-muted-foreground" title={run.checksum}>
                {run.checksum}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
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
      <span className={mono ? 'font-mono text-xs text-foreground truncate' : 'text-foreground'}>
        {value}
      </span>
    </div>
  )
}
