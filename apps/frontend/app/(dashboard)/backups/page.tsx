import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { BackupsTable } from '@/components/deploycore/backups/backups-table'
import { BackupRunsTable } from '@/components/deploycore/databases/database-backups-panel'
import { backupJobs, backupRuns } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function BackupsPage() {
  const jobs = getDemoFixtures(backupJobs)
  const runs = getDemoFixtures(backupRuns)
  const recentRuns = [...runs].sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1))

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Backups"
        description="Automated database backup schedules, retention, and run history."
      />
      <Card>
        <CardHeader>
          <CardTitle>Backup jobs</CardTitle>
          <CardDescription>Policies and destinations across database instances.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <BackupsTable jobs={jobs} runs={runs} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Recent runs</CardTitle>
          <CardDescription>
            Status, started, completed, duration, size, destination, and checksum.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <BackupRunsTable runs={recentRuns} showDatabase />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
