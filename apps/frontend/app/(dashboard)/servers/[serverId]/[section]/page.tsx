import { notFound } from 'next/navigation'
import Link from 'next/link'
import { Box, HardDrive, Network, Package } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ApplicationsTable } from '@/components/deploycore/applications/applications-table'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { EmptyState } from '@/components/platform/empty-state'
import { StatusBadge } from '@/components/platform/status-badge'
import { MaintenanceModeCard } from '@/components/deploycore/servers/maintenance-mode-card'
import { ServerMetricsChart } from '@/components/deploycore/servers/server-metrics-chart'
import { ServerAgentPanel, ServerSettingsPanel } from '@/components/deploycore/servers/server-overview'
import {
  findServer,
  getServerApplications,
  getServerContainers,
  getServerImages,
  getServerLogs,
  getServerNetworks,
  getServerVolumes,
  SERVER_SECTIONS,
  serverMetricSeries,
  type ServerSectionId,
} from '@/lib/servers'
import { servers } from '@/lib/mock-data'

const SECTION_IDS = new Set(SERVER_SECTIONS.map((s) => s.id))

export default async function ServerSectionPage({
  params,
}: {
  params: Promise<{ serverId: string; section: string }>
}) {
  const { serverId, section } = await params
  const server = findServer(serverId, servers)
  if (!server) notFound()
  if (!SECTION_IDS.has(section as ServerSectionId) || section === 'overview') notFound()

  if (section === 'applications') {
    const rows = getServerApplications(server)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Box}
                title="No applications"
                description="Applications placed on this server will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <ApplicationsTable applications={rows} listing />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'containers') {
    const rows = getServerContainers(server)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Box}
                title="No containers"
                description="Runtime containers on this host will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Container</TableHead>
                  <TableHead>Application</TableHead>
                  <TableHead>Revision</TableHead>
                  <TableHead>Image</TableHead>
                  <TableHead>CPU</TableHead>
                  <TableHead>Memory</TableHead>
                  <TableHead>Restarts</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((container) => (
                  <TableRow key={container.id}>
                    <TableCell className="font-mono text-xs">{container.name}</TableCell>
                    <TableCell>
                      <Link
                        href={`/applications/${container.applicationId}`}
                        className="hover:underline"
                      >
                        {container.application}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{container.revision}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {container.image}
                    </TableCell>
                    <TableCell className="tabular text-muted-foreground">{container.cpu}%</TableCell>
                    <TableCell className="tabular text-muted-foreground">
                      {container.memory}%
                    </TableCell>
                    <TableCell className="tabular text-muted-foreground">{container.restarts}</TableCell>
                    <TableCell>
                      <StatusBadge status={container.status} showDot />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'images') {
    const rows = getServerImages(server)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Package}
                title="No images"
                description="Images used by workloads on this server will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Repository</TableHead>
                  <TableHead>Tag</TableHead>
                  <TableHead>Digest</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Location</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((image) => (
                  <TableRow key={image.id}>
                    <TableCell className="font-mono text-xs">{image.name}</TableCell>
                    <TableCell className="font-mono text-xs">{image.tag}</TableCell>
                    <TableCell className="font-mono text-[11px] text-muted-foreground">
                      {image.digest}
                    </TableCell>
                    <TableCell className="tabular text-muted-foreground">{image.sizeMb} MB</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{image.location}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{image.createdAt}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'volumes') {
    const rows = getServerVolumes(server)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={HardDrive}
                title="No volumes"
                description="Persistent volumes on this server will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Volume</TableHead>
                  <TableHead>Driver</TableHead>
                  <TableHead>Attached</TableHead>
                  <TableHead>Mount</TableHead>
                  <TableHead>Usage</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((volume) => (
                  <TableRow key={volume.id}>
                    <TableCell className="font-medium">{volume.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {volume.driver}
                    </TableCell>
                    <TableCell className="text-sm">{volume.attachedResource}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {volume.mountPath}
                    </TableCell>
                    <TableCell className="tabular text-muted-foreground">
                      {volume.usedGb}/{volume.totalGb} GB
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={volume.status} showDot />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'networks') {
    const rows = getServerNetworks(server)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Network}
                title="No networks"
                description="Docker networks used by applications on this server will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Network</TableHead>
                  <TableHead>Driver</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Project</TableHead>
                  <TableHead>Services</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((network) => (
                  <TableRow key={network.id}>
                    <TableCell className="font-medium">{network.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {network.driver}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{network.scope}</TableCell>
                    <TableCell className="text-sm">{network.project}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {network.connectedServices.join(', ')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'metrics') {
    const series = serverMetricSeries(server, 36)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Host metrics</CardTitle>
          <CardDescription>CPU, memory, and disk utilisation samples.</CardDescription>
        </CardHeader>
        <CardContent>
          <ServerMetricsChart series={series} />
        </CardContent>
      </Card>
    )
  }

  if (section === 'logs') {
    return (
      <BuildLogViewer
        lines={getServerLogs(server, 120)}
        streaming={server.status !== 'offline'}
        title={`${server.name}-agent`}
      />
    )
  }

  if (section === 'agent') {
    return <ServerAgentPanel server={server} />
  }

  if (section === 'settings') {
    return (
      <div className="flex flex-col gap-4">
        <ServerSettingsPanel server={server} />
        <MaintenanceModeCard server={server} />
      </div>
    )
  }

  notFound()
}
