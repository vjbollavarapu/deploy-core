'use client'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { ApplicationsTable } from '@/components/deploycore/applications/applications-table'
import { CodeBlock } from '@/components/platform/code-block'
import { CopyButton } from '@/components/platform/copy-button'
import { DetailList } from '@/components/platform/detail-list'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { ServerMetricsChart } from '@/components/deploycore/servers/server-metrics-chart'
import {
  getServerApplications,
  getServerContainers,
  serverMetricSeries,
} from '@/lib/servers'
import type { Server } from '@/lib/types'

interface ServerOverviewProps {
  server: Server
}

export function ServerOverview({ server }: ServerOverviewProps) {
  const apps = getServerApplications(server).slice(0, 5)
  const containers = getServerContainers(server)
  const series = serverMetricSeries(server)

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2" size="sm">
          <CardHeader>
            <CardTitle>Resources</CardTitle>
            <CardDescription>Current utilization on this host.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <ResourceUsageBar label="CPU" value={server.cpu} detail={`${server.cpuCores} cores`} />
            <ResourceUsageBar
              label="Memory"
              value={server.memory}
              detail={`${server.memoryTotalGb} GB total`}
            />
            <ResourceUsageBar
              label="Disk"
              value={server.disk}
              detail={`${server.diskTotalGb} GB total`}
            />
            <Separator />
            <div className="grid grid-cols-3 gap-4 text-sm">
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Load average</span>
                <span className="font-mono text-foreground">{server.load.join(' / ')}</span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Containers</span>
                <span className="text-foreground">{containers.length} tracked</span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Uptime</span>
                <span className="text-foreground">{server.uptime}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Host details</CardTitle>
            <CardDescription>System and agent information.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            <DetailRow label="Public IP" value={server.ip} copyable />
            <DetailRow label="Private IP" value={server.privateIp} copyable />
            <DetailRow label="OS" value={server.os} />
            <DetailRow label="Architecture" value={server.arch} />
            <DetailRow label="Docker" value={server.dockerVersion} />
            <DetailRow label="Agent" value={server.agentVersion} />
            <DetailRow label="Last heartbeat" value={server.lastHeartbeat} />
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Recent utilisation</CardTitle>
          <CardDescription>CPU and memory over the last 24 samples.</CardDescription>
        </CardHeader>
        <CardContent>
          <ServerMetricsChart series={series} heightClassName="h-40" showDisk={false} />
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Hosted applications</CardTitle>
          <CardDescription>
            {apps.length === 0
              ? 'No applications are currently deployed on this server.'
              : `${apps.length} application${apps.length === 1 ? '' : 's'} on this host.`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {apps.length === 0 ? (
            <p className="px-4 pb-4 text-sm text-muted-foreground">Nothing to show yet.</p>
          ) : (
            <ApplicationsTable applications={apps} listing />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function DetailRow({
  label,
  value,
  copyable,
}: {
  label: string
  value: string
  copyable?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="flex items-center gap-1.5">
        <span className="font-mono text-foreground">{value}</span>
        {copyable ? <CopyButton value={value} /> : null}
      </div>
    </div>
  )
}

export function ServerAgentPanel({ server }: { server: Server }) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card size="sm">
        <CardHeader>
          <CardTitle>Agent</CardTitle>
          <CardDescription>Node agent process managed by DeployCore.</CardDescription>
        </CardHeader>
        <CardContent>
          <DetailList
            columns={2}
            items={[
              { label: 'Version', value: server.agentVersion },
              { label: 'Status', value: <StatusBadge status={server.status} /> },
              { label: 'Last heartbeat', value: server.lastHeartbeat },
              { label: 'Docker', value: server.dockerVersion },
              { label: 'OS', value: server.os },
              { label: 'Arch', value: server.arch },
            ]}
          />
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Reconnect</CardTitle>
          <CardDescription>
            If the agent loses connectivity, re-run registration with a fresh temporary token.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <CodeBlock
            code={`deploycore-agent status\nsystemctl status deploycore-agent`}
            label="Diagnostics"
          />
          <p className="text-xs text-muted-foreground">
            Prefer rotating tokens from Add server → Registration rather than reusing expired
            credentials.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

export function ServerSettingsPanel({ server }: { server: Server }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>General</CardTitle>
        <CardDescription>Identity and placement metadata for this host.</CardDescription>
      </CardHeader>
      <CardContent>
        <DetailList
          columns={2}
          items={[
            { label: 'Name', value: server.name },
            { label: 'Provider', value: server.provider },
            { label: 'Region', value: server.region },
            {
              label: 'Public IP',
              value: <span className="font-mono text-xs">{server.ip}</span>,
            },
          ]}
        />
      </CardContent>
    </Card>
  )
}
