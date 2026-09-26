import Link from 'next/link'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ProviderBadge } from '@/components/platform/provider-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { displayServerValue, UNAVAILABLE } from '@/lib/servers'
import type { Server } from '@/lib/types'

interface ServersTableProps {
  servers: Server[]
}

function UtilizationCell({
  value,
  suffix,
}: {
  value: number | null
  suffix: string
}) {
  if (value == null) {
    return <span className="text-xs text-muted-foreground">{UNAVAILABLE}</span>
  }
  return (
    <ResourceUsageBar
      value={value}
      detail={`${value}%${suffix ? ` · ${suffix}` : ''}`}
      size="sm"
    />
  )
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
            <TableCell className="font-mono text-xs text-muted-foreground">
              {displayServerValue(server.region)}
            </TableCell>
            <TableCell className="font-mono text-xs">{displayServerValue(server.ip)}</TableCell>
            <TableCell className="min-w-28">
              <UtilizationCell
                value={server.cpu}
                suffix={server.cpuCores != null ? `${server.cpuCores}c` : ''}
              />
            </TableCell>
            <TableCell className="min-w-28">
              <UtilizationCell
                value={server.memory}
                suffix={server.memoryTotalGb != null ? `${server.memoryTotalGb}G` : ''}
              />
            </TableCell>
            <TableCell className="min-w-28">
              <UtilizationCell
                value={server.disk}
                suffix={server.diskTotalGb != null ? `${server.diskTotalGb}G` : ''}
              />
            </TableCell>
            <TableCell className="tabular text-muted-foreground">
              {displayServerValue(server.containers)}
            </TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">
              {displayServerValue(server.agentVersion)}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {server.lastHeartbeat ?? 'No heartbeat yet'}
            </TableCell>
            <TableCell>
              <StatusBadge status={server.status} showDot />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
