import { Card, CardContent } from '@/components/ui/card'
import { ServersTable } from '@/components/deploycore/servers/servers-table'
import { servers } from '@/lib/mock-data'

export default function AdminServersPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <ServersTable servers={servers} />
      </CardContent>
    </Card>
  )
}
