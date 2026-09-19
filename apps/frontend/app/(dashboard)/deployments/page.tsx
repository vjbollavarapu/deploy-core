import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { DeploymentsFilterTable } from '@/components/deploycore/deployments/deployments-filter-table'
import { deployments } from '@/lib/mock-data'

export default function DeploymentsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Deployments"
        description="Every deployment across all projects and environments, most recent first."
      />
      <Card>
        <CardContent className="p-0">
          <DeploymentsFilterTable deployments={deployments} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
