import { Card, CardContent } from '@/components/ui/card'
import { PlatformJobsTable } from '@/components/deploycore/super-admin/platform-jobs-table'
import { platformJobs } from '@/lib/mock-data'

export default function AdminJobsPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <PlatformJobsTable jobs={platformJobs} />
      </CardContent>
    </Card>
  )
}
