import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { HealthChecksTable } from '@/components/deploycore/health-checks/health-checks-table'
import { healthChecks } from '@/lib/mock-data'

export default function HealthChecksPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Health Checks"
        description="Synthetic monitors probing each application's uptime and latency."
      />
      <Card>
        <CardContent className="p-0">
          <HealthChecksTable checks={healthChecks} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
