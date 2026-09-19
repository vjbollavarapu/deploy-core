import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { AuditLogTable } from '@/components/deploycore/audit/audit-log-table'
import { auditLog } from '@/lib/mock-data'

export default function AuditLogPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Audit log"
        description="Immutable record of organization actions. Secret values are never shown."
      />
      <Card>
        <CardHeader>
          <CardTitle>Events</CardTitle>
          <CardDescription>
            Timestamp, actor, action, resource, project, IP, and result. Open a row for before/after
            metadata.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <AuditLogTable entries={auditLog} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
