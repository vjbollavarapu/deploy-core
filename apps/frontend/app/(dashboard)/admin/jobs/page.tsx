import { Card, CardContent } from '@/components/ui/card'
import { PlatformJobsTable } from '@/components/deploycore/super-admin/platform-jobs-table'
import { platformJobs as rawPlatformJobs } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const platformJobs = getDemoFixtures(rawPlatformJobs)

export default function AdminJobsPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <PlatformJobsTable jobs={platformJobs} />
      </CardContent>
    </Card>
  )
}
