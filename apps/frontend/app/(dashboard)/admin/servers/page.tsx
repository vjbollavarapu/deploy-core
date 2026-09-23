import { Card, CardContent } from '@/components/ui/card'
import { ServersTable } from '@/components/deploycore/servers/servers-table'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)

export default function AdminServersPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <ServersTable servers={servers} />
      </CardContent>
    </Card>
  )
}
