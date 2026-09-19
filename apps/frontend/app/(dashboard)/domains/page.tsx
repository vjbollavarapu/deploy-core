import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { DomainsTable } from '@/components/deploycore/domains/domains-table'
import { domains } from '@/lib/mock-data'

export default function DomainsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Domains"
        description="Custom domains, DNS verification, and TLS certificate lifecycle across every application."
      />
      <Card>
        <CardContent className="p-0">
          <DomainsTable domains={domains} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
