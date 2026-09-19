import { notFound } from 'next/navigation'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { DatabaseBackupsPanel } from '@/components/deploycore/databases/database-backups-panel'
import { DatabaseConnectionPanel } from '@/components/deploycore/databases/database-connection-panel'
import { DatabaseMetricsChart } from '@/components/deploycore/databases/database-metrics-chart'
import { DatabaseRestorePanel } from '@/components/deploycore/databases/database-restore-panel'
import { DatabaseSettingsPanel } from '@/components/deploycore/databases/database-overview'
import {
  DATABASE_SECTIONS,
  databaseMetricSeries,
  findDatabase,
  getDatabaseLogs,
  type DatabaseSectionId,
} from '@/lib/databases'
import { databases } from '@/lib/mock-data'

const SECTION_IDS = new Set(DATABASE_SECTIONS.map((s) => s.id))

export default async function DatabaseSectionPage({
  params,
}: {
  params: Promise<{ databaseId: string; section: string }>
}) {
  const { databaseId, section } = await params
  const database = findDatabase(databaseId, databases)
  if (!database) notFound()
  if (!SECTION_IDS.has(section as DatabaseSectionId) || section === 'overview') notFound()

  if (section === 'connection') {
    return <DatabaseConnectionPanel database={database} />
  }

  if (section === 'metrics') {
    const series = databaseMetricSeries(database, 36)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Database metrics</CardTitle>
          <CardDescription>CPU, connections, and storage utilisation samples.</CardDescription>
        </CardHeader>
        <CardContent>
          <DatabaseMetricsChart series={series} />
        </CardContent>
      </Card>
    )
  }

  if (section === 'backups') {
    return <DatabaseBackupsPanel database={database} />
  }

  if (section === 'restore') {
    return <DatabaseRestorePanel database={database} />
  }

  if (section === 'logs') {
    return (
      <BuildLogViewer
        lines={getDatabaseLogs(database, 120)}
        streaming={database.status === 'healthy' || database.status === 'degraded'}
        title={`${database.name}-postgres`}
      />
    )
  }

  if (section === 'settings') {
    return <DatabaseSettingsPanel database={database} />
  }

  notFound()
}
