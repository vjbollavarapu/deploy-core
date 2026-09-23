import { Card, CardContent } from '@/components/ui/card'
import { HealthChecksTable } from '@/components/deploycore/health-checks/health-checks-table'
import { healthChecks as rawHealthChecks } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const healthChecks = getDemoFixtures(rawHealthChecks)

export default function AdminHealthPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <HealthChecksTable checks={healthChecks} />
      </CardContent>
    </Card>
  )
}
