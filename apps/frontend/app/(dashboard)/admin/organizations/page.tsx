import { Card, CardContent } from '@/components/ui/card'
import { OrganizationsTable } from '@/components/deploycore/super-admin/organizations-table'
import { organizations as rawOrganizations } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const organizations = getDemoFixtures(rawOrganizations)

export default function AdminOrganizationsPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <OrganizationsTable organizations={organizations} />
      </CardContent>
    </Card>
  )
}
