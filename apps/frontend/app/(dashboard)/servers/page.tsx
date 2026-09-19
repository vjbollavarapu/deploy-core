import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { AddServerWizard } from '@/components/deploycore/servers/add-server-wizard'
import { ServersTable } from '@/components/deploycore/servers/servers-table'
import { servers } from '@/lib/mock-data'

export default function ServersPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Servers"
        description="Physical and virtual hosts running your containers."
        actions={<AddServerWizard />}
      />
      <Card>
        <CardContent className="p-0">
          <ServersTable servers={servers} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
