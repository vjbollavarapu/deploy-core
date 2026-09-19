import { Card, CardContent } from '@/components/ui/card'
import { HealthChecksTable } from '@/components/deploycore/health-checks/health-checks-table'
import { healthChecks } from '@/lib/mock-data'

export default function AdminHealthPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <HealthChecksTable checks={healthChecks} />
      </CardContent>
    </Card>
  )
}
