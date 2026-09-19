'use client'

import Link from 'next/link'
import { Area, AreaChart } from 'recharts'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import {
  applicationMetricSeries,
  getApplicationMetricRows,
} from '@/lib/observability'
import type { Application } from '@/lib/types'

const chartConfig = {
  cpu: { label: 'CPU', color: 'var(--chart-1)' },
  memory: { label: 'RAM', color: 'var(--chart-2)' },
} satisfies ChartConfig

export function ApplicationMetricsPanel({ application }: { application: Application }) {
  const metrics = getApplicationMetricRows(application)
  const series = applicationMetricSeries(application)

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <MetricCard
          label="CPU"
          value={`${metrics.cpu}%`}
          detail={`Limit ${metrics.cpuLimit} cores`}
        />
        <MetricCard
          label="RAM"
          value={`${metrics.ram}%`}
          detail={`Limit ${metrics.memoryLimit} MB`}
        />
        <MetricCard label="Storage" value="—" detail="Not on application API" />
        <MetricCard label="Network" value="—" detail="Not provided by API" />
        <MetricCard label="Restart count" value={String(metrics.restarts)} detail="Container sum" />
        <MetricCard label="Uptime" value={metrics.uptime} />
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>CPU & RAM</CardTitle>
          <CardDescription>
            Series seeded from current application.cpu / application.memory values only.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ChartContainer config={chartConfig} className="aspect-auto h-48 w-full">
            <AreaChart data={series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
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
            </AreaChart>
          </ChartContainer>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Containers</CardTitle>
          <CardDescription>
            Per-container CPU, RAM, and restart count. Open{' '}
            <Link href={`/applications/${application.id}/logs`} className="hover:underline">
              logs
            </Link>{' '}
            for correlated output.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {metrics.containers.length === 0 ? (
            <p className="px-4 pb-4 text-sm text-muted-foreground">No containers for this application.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Container</TableHead>
                  <TableHead>Revision</TableHead>
                  <TableHead>CPU</TableHead>
                  <TableHead>RAM</TableHead>
                  <TableHead>Restarts</TableHead>
                  <TableHead>Network</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {metrics.containers.map((container) => (
                  <TableRow key={container.id}>
                    <TableCell className="font-mono text-xs">{container.name}</TableCell>
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
          )}
        </CardContent>
      </Card>
    </div>
  )
}
