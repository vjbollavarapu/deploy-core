import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)

export default function AdminAgentsPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Server</TableHead>
              <TableHead>Agent</TableHead>
              <TableHead>Docker</TableHead>
              <TableHead>Heartbeat</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {servers.map((server) => (
              <TableRow key={server.id}>
                <TableCell className="font-medium">{server.name}</TableCell>
                <TableCell className="font-mono text-xs">{server.agentVersion ?? '—'}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {server.dockerVersion ?? '—'}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {server.lastHeartbeat ?? 'No heartbeat yet'}
                </TableCell>
                <TableCell>
                  <StatusBadge status={server.status} showDot />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
