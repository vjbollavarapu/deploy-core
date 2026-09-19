import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { NetworksTable } from '@/components/deploycore/networks/networks-table'
import { networks } from '@/lib/mock-data'

export default function NetworksPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Networks"
        description="Docker networks connecting services within each project and environment."
      />
      <Card>
        <CardContent className="p-0">
          <NetworksTable networks={networks} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
