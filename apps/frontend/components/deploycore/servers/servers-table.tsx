import Link from 'next/link'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ProviderBadge } from '@/components/platform/provider-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import type { Server } from '@/lib/types'

interface ServersTableProps {
  servers: Server[]
}

export function ServersTable({ servers }: ServersTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Server</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Region</TableHead>
          <TableHead>IP</TableHead>
          <TableHead>CPU</TableHead>
          <TableHead>Memory</TableHead>
          <TableHead>Disk</TableHead>
          <TableHead>Containers</TableHead>
          <TableHead>Agent</TableHead>
          <TableHead>Last heartbeat</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {servers.map((server) => (
          <TableRow key={server.id}>
            <TableCell>
              <Link
                href={`/servers/${server.id}`}
                className="font-medium text-foreground hover:underline"
              >
                {server.name}
              </Link>
            </TableCell>
            <TableCell>
              <ProviderBadge provider={server.provider} />
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{server.region}</TableCell>
            <TableCell className="font-mono text-xs">{server.ip}</TableCell>
            <TableCell className="min-w-28">
              <ResourceUsageBar
                value={server.cpu}
                detail={`${server.cpu}% · ${server.cpuCores}c`}
                size="sm"
              />
            </TableCell>
            <TableCell className="min-w-28">
              <ResourceUsageBar
                value={server.memory}
                detail={`${server.memory}% · ${server.memoryTotalGb}G`}
                size="sm"
              />
            </TableCell>
            <TableCell className="min-w-28">
              <ResourceUsageBar
                value={server.disk}
                detail={`${server.disk}% · ${server.diskTotalGb}G`}
                size="sm"
              />
            </TableCell>
            <TableCell className="tabular text-muted-foreground">{server.containers}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">
              {server.agentVersion}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">{server.lastHeartbeat}</TableCell>
            <TableCell>
              <StatusBadge status={server.status} showDot />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
