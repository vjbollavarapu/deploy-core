'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import { ApplicationSourceValue } from '@/components/deploycore/applications/application-source-value'
import { useProductionApplication } from '@/components/deploycore/applications/production-application-shell'
import { useApplicationSourceRows } from '@/components/deploycore/applications/use-application-source-display'
import { getStatusConfig } from '@/lib/status'
import type { Status } from '@/lib/types'

function show(value: string | number | null | undefined): string {
  if (value == null) return '—'
  const text = String(value).trim()
  return text.length > 0 ? text : '—'
}

export function ProductionApplicationOverview() {
  const application = useProductionApplication()
  const sourceRows = useApplicationSourceRows(application, 'summary')
  if (!application) return null

  const statusLabel = getStatusConfig(application.status as Status).label

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4">
        <MetricCard label="Status" value={statusLabel} />
        <MetricCard label="Type" value={application.type} />
        <MetricCard label="Replicas" value={show(application.desiredReplicas)} />
        <MetricCard label="Slug" value={show(application.slug)} />
      </div>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <DetailList
            columns={2}
            items={[
              { label: 'Name', value: application.name },
              { label: 'Application ID', value: <span className="font-mono text-xs">{application.id}</span> },
              {
                label: 'Status',
                value: <StatusBadge status={application.status as Status} />,
              },
              { label: 'Project', value: show(application.projectName ?? application.projectId) },
              { label: 'Environment', value: show(application.environmentName ?? application.environmentId) },
              { label: 'Server', value: show(application.serverName ?? application.serverId) },
              ...sourceRows.map((row) => ({
                label: row.label,
                value: <ApplicationSourceValue row={row} />,
              })),
              {
                label: 'CPU limit',
                value: application.cpuLimitMillis == null ? '—' : `${application.cpuLimitMillis} ms`,
              },
              {
                label: 'Memory limit',
                value: application.memoryLimitBytes == null ? '—' : `${application.memoryLimitBytes} bytes`,
              },
            ]}
          />
        </CardContent>
      </Card>
    </div>
  )
}
