'use client'

import Link from 'next/link'
import { Area, AreaChart } from 'recharts'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import { applications } from '@/lib/mock-data'
import {
  getContainerMetricRows,
  getFleetMetricSummaries,
  getServerMetricRows,
} from '@/lib/observability'

const chartConfig = {
  cpu: { label: 'CPU', color: 'var(--chart-1)' },
  memory: { label: 'RAM', color: 'var(--chart-2)' },
  storage: { label: 'Storage', color: 'var(--chart-3)' },
} satisfies ChartConfig

export function MetricsDashboard() {
  const summary = getFleetMetricSummaries()
  const serverRows = getServerMetricRows()
  const containerRows = getContainerMetricRows()

  const fleetSeries = Array.from({ length: 24 }).map((_, index) => {
    const wave = Math.sin(index / 3) * 4
    return {
      t: `${24 - index}m`,
      cpu: Math.min(100, Math.max(0, Math.round(summary.cpu + wave))),
      memory: Math.min(100, Math.max(0, Math.round(summary.ram + wave * 0.7))),
      storage: Math.min(100, Math.max(0, Math.round(summary.storage + (index % 3) - 1))),
    }
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <MetricCard label="CPU (apps avg)" value={`${summary.cpu}%`} detail="From application.cpu" />
        <MetricCard label="RAM (apps avg)" value={`${summary.ram}%`} detail="From application.memory" />
        <MetricCard
          label="Storage (hosts avg)"
          value={`${summary.storage}%`}
          detail="From server.disk"
        />
        <MetricCard
          label="Network"
          value="—"
          detail="Not provided by API"
          tone="warning"
        />
        <MetricCard
          label="Restart count"
          value={String(summary.restarts)}
          detail="Sum of container.restarts"
        />
        <MetricCard
          label="Tracked uptime"
          value={`${summary.applicationCount} apps`}
          detail={`${summary.serverCount} servers · ${summary.containerCount} containers`}
        />
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Fleet utilisation</CardTitle>
          <CardDescription>
            CPU, RAM, and storage derived from current API entity values. Network RX/TX is omitted
            because it is not on the mock API payloads.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ChartContainer config={chartConfig} className="aspect-auto h-56 w-full">
            <AreaChart data={fleetSeries} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
              <Area
                type="monotone"
                dataKey="cpu"
                stroke="var(--color-cpu)"
                fill="var(--color-cpu)"
                fillOpacity={0.12}
                strokeWidth={1.5}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="memory"
                stroke="var(--color-memory)"
                fill="var(--color-memory)"
                fillOpacity={0.1}
                strokeWidth={1.5}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="storage"
                stroke="var(--color-storage)"
                fill="var(--color-storage)"
                fillOpacity={0.08}
                strokeWidth={1.5}
                isAnimationActive={false}
              />
            </AreaChart>
          </ChartContainer>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Servers</CardTitle>
            <CardDescription>CPU, RAM, storage, and uptime from server records.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Server</TableHead>
                  <TableHead>CPU</TableHead>
                  <TableHead>RAM</TableHead>
                  <TableHead>Storage</TableHead>
                  <TableHead>Network</TableHead>
                  <TableHead>Uptime</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {serverRows.map((server) => (
                  <TableRow key={server.id}>
                    <TableCell>
                      <Link href={`/servers/${server.id}/metrics`} className="hover:underline">
                        {server.name}
                      </Link>
                    </TableCell>
                    <TableCell className="tabular">{server.cpu}%</TableCell>
                    <TableCell className="tabular">{server.ram}%</TableCell>
                    <TableCell className="tabular">{server.storage}%</TableCell>
                    <TableCell className="text-muted-foreground">—</TableCell>
                    <TableCell className="text-muted-foreground">{server.uptime}</TableCell>
                    <TableCell>
                      <StatusBadge status={server.status} showDot />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Applications</CardTitle>
            <CardDescription>CPU, RAM, and uptime from application records.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Application</TableHead>
                  <TableHead>CPU</TableHead>
                  <TableHead>RAM</TableHead>
                  <TableHead>Network</TableHead>
                  <TableHead>Uptime</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {applications.map((app) => (
                  <TableRow key={app.id}>
                    <TableCell>
                      <Link href={`/applications/${app.id}/metrics`} className="hover:underline">
                        {app.name}
                      </Link>
                    </TableCell>
                    <TableCell className="tabular">{app.cpu}%</TableCell>
                    <TableCell className="tabular">{app.memory}%</TableCell>
                    <TableCell className="text-muted-foreground">—</TableCell>
                    <TableCell className="text-muted-foreground">{app.uptime}</TableCell>
                    <TableCell>
                      <StatusBadge status={app.status} showDot />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Containers</CardTitle>
          <CardDescription>
            CPU, RAM, and restart count from container records. Storage and network are not on the
            container API payload.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Container</TableHead>
                <TableHead>Application</TableHead>
                <TableHead>Revision</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>RAM</TableHead>
                <TableHead>Restarts</TableHead>
                <TableHead>Network</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {containerRows.map((container) => (
                <TableRow key={container.id}>
                  <TableCell className="font-mono text-xs">{container.name}</TableCell>
                  <TableCell>
                    <Link
                      href={`/applications/${container.applicationId}/metrics`}
                      className="hover:underline"
                    >
                      {container.application}
                    </Link>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{container.revision}</TableCell>
                  <TableCell className="tabular">{container.cpu}%</TableCell>
                  <TableCell className="tabular">{container.memory}%</TableCell>
                  <TableCell className="tabular">{container.restarts}</TableCell>
                  <TableCell className="text-muted-foreground">—</TableCell>
                  <TableCell>
                    <StatusBadge status={container.status} showDot />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
