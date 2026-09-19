import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { RevisionsTable } from '@/components/deploycore/revisions/revisions-table'
import { revisions } from '@/lib/mock-data'

export default function RevisionsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Revisions"
        description="Immutable builds across applications, traffic splits, rollbacks, and configuration compare."
      />
      <Card>
        <CardContent className="p-0">
          <RevisionsTable revisions={revisions} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
