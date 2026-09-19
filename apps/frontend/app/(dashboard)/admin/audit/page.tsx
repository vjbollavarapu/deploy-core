import { Card, CardContent } from '@/components/ui/card'
import { AuditLogTable } from '@/components/deploycore/audit/audit-log-table'
import { auditLog } from '@/lib/mock-data'

export default function AdminAuditPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <AuditLogTable entries={auditLog} />
      </CardContent>
    </Card>
  )
}
