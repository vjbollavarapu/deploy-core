import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { ContainersTable } from '@/components/deploycore/containers/containers-table'
import { containers } from '@/lib/mock-data'

export default function ContainersPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Containers"
        description="Every container across the fleet — inspect, restart, stop, or remove with confirmation."
      />
      <Card>
        <CardContent className="p-0">
          <ContainersTable containers={containers} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
