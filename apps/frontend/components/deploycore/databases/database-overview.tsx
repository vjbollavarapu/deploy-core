'use client'

import Link from 'next/link'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { DetailList } from '@/components/platform/detail-list'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { DatabaseMetricsChart } from '@/components/deploycore/databases/database-metrics-chart'
import {
  databaseMetricSeries,
  getDatabaseBackupJob,
  getDatabaseBackupRuns,
} from '@/lib/databases'
import { servers } from '@/lib/mock-data'
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
                      : run.status === 'running'
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
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>General</CardTitle>
        <CardDescription>Identity and placement for this database instance.</CardDescription>
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
            {
              label: 'Credential policy',
              value: database.credentialsRevealAllowed ? 'Reveal allowed' : 'Reveal blocked',
            },
          ]}
        />
      </CardContent>
    </Card>
  )
}
