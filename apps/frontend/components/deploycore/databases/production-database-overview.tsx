'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { StatusBadge } from '@/components/platform/status-badge'
import { useProductionDatabase } from '@/components/deploycore/databases/production-database-shell'
import type { Status } from '@/lib/types'

export function ProductionDatabaseOverview() {
  const database = useProductionDatabase()
  if (!database) return null

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Overview</CardTitle>
      </CardHeader>
      <CardContent>
        <DetailList
          columns={2}
          items={[
            { label: 'Name', value: database.name },
            { label: 'Database ID', value: <span className="font-mono text-xs">{database.id}</span> },
            { label: 'Status', value: <StatusBadge status={database.status as Status} /> },
            { label: 'Engine', value: `${database.type} ${database.version}` },
            { label: 'Project', value: database.project },
            { label: 'Environment', value: database.environment },
            { label: 'Server', value: database.server },
            { label: 'Database name', value: <span className="font-mono text-xs">{database.dbName}</span> },
            { label: 'Username', value: <span className="font-mono text-xs">{database.username}</span> },
            { label: 'Private host', value: <span className="font-mono text-xs">{database.connectionHost}</span> },
            { label: 'Port', value: database.port > 0 ? String(database.port) : '—' },
            { label: 'Storage volume', value: database.storageVolumeName || '—' },
          ]}
        />
      </CardContent>
    </Card>
  )
}
