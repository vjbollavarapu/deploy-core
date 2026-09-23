'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { DetailList } from '@/components/platform/detail-list'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { DatabaseMetricsChart } from '@/components/deploycore/databases/database-metrics-chart'
import { databaseMetricSeries, getDatabaseBackupJob, getDatabaseBackupRuns } from '@/lib/databases'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)
import type { DatabaseInstance } from '@/lib/types'

interface DatabaseOverviewProps {
  database: DatabaseInstance
}

export function DatabaseOverview({ database }: DatabaseOverviewProps) {
  const storagePercent = Math.round((database.storageUsedGb / database.storageTotalGb) * 100)
  const hostServer = servers.find((s) => s.name === database.server)
  const job = getDatabaseBackupJob(database)
  const recentRuns = getDatabaseBackupRuns(database).slice(0, 3)
  const series = databaseMetricSeries(database)

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2" size="sm">
          <CardHeader>
            <CardTitle>Storage</CardTitle>
            <CardDescription>Allocated disk for this instance.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <ResourceUsageBar
              label="Used"
              value={storagePercent}
              detail={`${database.storageUsedGb} / ${database.storageTotalGb} GB`}
            />
            <Separator />
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Engine</span>
                <span className="font-mono text-xs text-foreground">
                  {database.type} {database.version}
                </span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Environment</span>
                <EnvironmentBadge environment={database.environment} />
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Server</span>
                {hostServer ? (
                  <Link href={`/servers/${hostServer.id}`} className="hover:underline">
                    {database.server}
                  </Link>
                ) : (
                  <span>{database.server}</span>
                )}
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Last backup</span>
                <span>{database.lastBackup}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Instance</CardTitle>
            <CardDescription>Placement and backup summary.</CardDescription>
          </CardHeader>
          <CardContent>
            <DetailList
              columns={1}
              items={[
                { label: 'Project', value: database.project },
                { label: 'Status', value: <StatusBadge status={database.status} showDot /> },
                { label: 'Snapshots', value: String(database.backups) },
                {
                  label: 'Policy',
                  value: job?.policy ?? 'Not configured',
                },
              ]}
            />
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Recent utilisation</CardTitle>
          <CardDescription>CPU, connections, and storage percentage samples.</CardDescription>
        </CardHeader>
        <CardContent>
          <DatabaseMetricsChart series={series} heightClassName="h-40" />
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>Recent backups</CardTitle>
            <CardDescription>Latest runs for this database.</CardDescription>
          </div>
          <Link
            href={`/databases/${database.id}/backups`}
            className="text-xs text-muted-foreground hover:text-foreground hover:underline"
          >
            View all
          </Link>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {recentRuns.length === 0 ? (
            <p className="text-sm text-muted-foreground">No backup runs yet.</p>
          ) : (
            recentRuns.map((run) => (
              <div
                key={run.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-2 text-sm"
              >
                <span className="font-mono text-xs text-muted-foreground">{run.startedAt}</span>
                <span className="tabular text-muted-foreground">{run.duration}</span>
                <span className="tabular text-muted-foreground">
                  {run.sizeGb > 0 ? `${run.sizeGb} GB` : '—'}
                </span>
                <StatusBadge
                  status={
                    run.status === 'success'
                      ? 'healthy'
                      : run.status === 'running' || run.status === 'queued'
                        ? 'running'
                        : 'failed'
                  }
                  showDot
                />
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export function DatabaseSettingsPanel({ database }: { database: DatabaseInstance }) {
  const router = useRouter()
  const [revealAllowed, setRevealAllowed] = useState(database.credentialsRevealAllowed)
  const [deleteOpen, setDeleteOpen] = useState(false)

  function toggleReveal(checked: boolean) {
    setRevealAllowed(checked)
    toast.success(
      checked
        ? 'Credential reveal allowed via API policy'
        : 'Credential reveal locked by API policy',
    )
  }

  function confirmDelete() {
    toast.success(`Database ${database.name} permanently deleted`)
    router.push('/databases')
  }

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader>
          <CardTitle>General configuration</CardTitle>
          <CardDescription>Identity, placement, and network endpoint metadata.</CardDescription>
        </CardHeader>
        <CardContent>
          <DetailList
            columns={2}
            items={[
              { label: 'Name', value: database.name },
              { label: 'Engine', value: `${database.type} ${database.version}` },
              { label: 'Project', value: database.project },
              { label: 'Environment', value: <EnvironmentBadge environment={database.environment} /> },
              { label: 'Server', value: database.server },
              { label: 'Host', value: <span className="font-mono text-xs">{database.connectionHost}</span> },
              { label: 'Port', value: String(database.port) },
              { label: 'Logical DB', value: database.dbName },
            ]}
          />
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Credential security policy</CardTitle>
          <CardDescription>
            Enforce control-plane policy locks on whether live connection passwords can be decrypted by operators.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between gap-4 rounded-lg border border-border p-3.5 bg-muted/20">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">Allow credential reveal</span>
              <p className="text-xs text-muted-foreground">
                When enabled, operators with proper role permissions can view connection credentials with audit logging.
              </p>
            </div>
            <Switch checked={revealAllowed} onCheckedChange={toggleReveal} />
          </div>
        </CardContent>
      </Card>

      <Card size="sm" className="border-destructive/30">
        <CardHeader>
          <CardTitle className="text-destructive">Danger zone</CardTitle>
          <CardDescription>
            Irreversible operations on this managed database instance.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">Delete database</span>
              <p className="text-xs text-muted-foreground">
                Permanently terminates the database container and detaches persistent volumes.
              </p>
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 data-icon="inline-start" />
              Delete database
            </Button>
          </div>
        </CardContent>
      </Card>

      <DestructiveConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={`Delete database ${database.name}?`}
        description={`This permanently destroys the database ${database.name} and unlinks its storage volumes. This action cannot be undone. Type the database name to confirm.`}
        confirmLabel="Delete permanently"
        confirmationPhrase={database.name}
        onConfirm={confirmDelete}
      />
    </div>
  )
}
