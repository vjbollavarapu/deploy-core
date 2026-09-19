import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { DatabasesTable } from '@/components/deploycore/databases/databases-table'
import { databases } from '@/lib/mock-data'

export default function DatabasesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Databases"
        description="Managed database instances across all projects. PostgreSQL is the initial engine."
      />
      <Card>
        <CardContent className="p-0">
          <DatabasesTable databases={databases} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
