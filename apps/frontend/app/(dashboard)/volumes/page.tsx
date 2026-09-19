import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { VolumesTable } from '@/components/deploycore/volumes/volumes-table'
import { volumes } from '@/lib/mock-data'

export default function VolumesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Volumes"
        description="Persistent storage volumes attached to applications and databases. Deletes require confirmation."
      />
      <Card>
        <CardContent className="p-0">
          <VolumesTable volumes={volumes} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
