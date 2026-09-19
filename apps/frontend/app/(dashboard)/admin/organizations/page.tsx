import { Card, CardContent } from '@/components/ui/card'
import { OrganizationsTable } from '@/components/deploycore/super-admin/organizations-table'
import { organizations } from '@/lib/mock-data'

export default function AdminOrganizationsPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <OrganizationsTable organizations={organizations} />
      </CardContent>
    </Card>
  )
}
