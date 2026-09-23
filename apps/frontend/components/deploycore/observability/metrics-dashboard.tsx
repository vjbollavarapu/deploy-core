'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { BarChart3 } from 'lucide-react'
import { Area, AreaChart } from 'recharts'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState } from '@/components/platform/empty-state'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient, ApiError } from '@/lib/api'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import {
  getContainerMetricRows,
  getFleetMetricSummaries,
  getServerMetricRows,
} from '@/lib/observability'
import type { Status } from '@/lib/types'

const chartConfig = {
  cpu: { label: 'CPU', color: 'var(--chart-1)' },
  memory: { label: 'RAM', color: 'var(--chart-2)' },
  storage: { label: 'Storage', color: 'var(--chart-3)' },
} satisfies ChartConfig

type ServerMetricSnap = {
  cpuPercent?: number | null
  memoryUsedBytes?: number | null
  memoryTotalBytes?: number | null
  diskUsedBytes?: number | null
  diskTotalBytes?: number | null
  source?: string
  recordedAt?: string
}

type ServerRow = {
  id: string
  name: string
  status: string
  metrics?: ServerMetricSnap | null
  error?: string
}

type Page<T> = { items: T[]; totalCount?: number }

function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || total == null || total <= 0) return null
  return Math.min(100, Math.max(0, Math.round((used / total) * 100)))
}

export function MetricsDashboard() {
  const demo = isDemoModeEnabled()
  const [loading, setLoading] = useState(!demo)
  const [error, setError] = useState<string | null>(null)
  const [serverRows, setServerRows] = useState<ServerRow[]>([])

  useEffect(() => {
    if (demo) {
      return
    }
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setError(null)
      try {
        const page = await apiClient.get<Page<{ id: string; name: string; status: string }>>(
          '/servers?limit=50',
        )
        const items = page.items ?? []
        const rows: ServerRow[] = await Promise.all(
          items.map(async (s) => {
            try {
              const res = await apiClient.get<{ metrics: ServerMetricSnap }>(
                `/servers/${s.id}/metrics`,
              )
              return { id: s.id, name: s.name, status: s.status, metrics: res.metrics }
            } catch (err) {
              const message = err instanceof ApiError ? err.message : 'metrics unavailable'
              return { id: s.id, name: s.name, status: s.status, metrics: null, error: message }
            }
          }),
        )
        if (!cancelled) setServerRows(rows)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load server metrics')
          setServerRows([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [demo])

  if (demo) {
    const summary = getFleetMetricSummaries()
    const demoServers = getServerMetricRows()
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
        <p className="text-sm text-muted-foreground">
          Demo mode: showing fixture metrics. Disable NEXT_PUBLIC_DEMO_MODE for Control Plane data.
        </p>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
          <MetricCard label="CPU (apps avg)" value={`${summary.cpu}%`} />
          <MetricCard label="RAM (apps avg)" value={`${summary.ram}%`} />
          <MetricCard label="Storage (hosts avg)" value={`${summary.storage}%`} />
          <MetricCard label="Network" value="—" detail="Not provided" tone="warning" />
          <MetricCard label="Restart count" value={String(summary.restarts)} />
          <MetricCard
            label="Tracked uptime"
            value={`${summary.applicationCount} apps`}
            detail={`${summary.serverCount} servers · ${containerRows.length} containers`}
          />
        </div>
        <Card size="sm">
          <CardHeader>
            <CardTitle>Fleet utilisation (demo)</CardTitle>
          </CardHeader>
          <CardContent>
            <ChartContainer config={chartConfig} className="aspect-auto h-56 w-full">
              <AreaChart data={fleetSeries} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <Area type="monotone" dataKey="cpu" stroke="var(--color-cpu)" fill="var(--color-cpu)" fillOpacity={0.12} strokeWidth={1.5} isAnimationActive={false} />
                <Area type="monotone" dataKey="memory" stroke="var(--color-memory)" fill="var(--color-memory)" fillOpacity={0.1} strokeWidth={1.5} isAnimationActive={false} />
                <Area type="monotone" dataKey="storage" stroke="var(--color-storage)" fill="var(--color-storage)" fillOpacity={0.08} strokeWidth={1.5} isAnimationActive={false} />
              </AreaChart>
            </ChartContainer>
          </CardContent>
        </Card>
        <Card size="sm">
          <CardHeader>
            <CardTitle>Servers (demo fixtures)</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {demoServers.length === 0 ? (
              <div className="p-6 text-center text-sm text-muted-foreground">No demo servers.</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Server</TableHead>
                    <TableHead>CPU</TableHead>
                    <TableHead>RAM</TableHead>
                    <TableHead>Storage</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {demoServers.map((server) => (
                    <TableRow key={server.id}>
                      <TableCell>
                        <Link href={`/servers/${server.id}/metrics`} className="hover:underline">
                          {server.name}
                        </Link>
                      </TableCell>
                      <TableCell className="tabular">{server.cpu}%</TableCell>
                      <TableCell className="tabular">{server.ram}%</TableCell>
                      <TableCell className="tabular">{server.storage}%</TableCell>
                      <TableCell>
                        <StatusBadge status={server.status} showDot />
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

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading metrics from Control Plane…</p>
  }

  if (error) {
    return (
      <EmptyState
        icon={BarChart3}
        title="Metrics unavailable"
        description={error}
      />
    )
  }

  if (serverRows.length === 0) {
    return (
      <EmptyState
        icon={BarChart3}
        title="No server metrics yet"
        description="Connect an Agent and wait for heartbeat or continuous metrics ingest (POST /agents/metrics)."
      />
    )
  }

  const cpuVals = serverRows
    .map((r) => r.metrics?.cpuPercent)
    .filter((v): v is number => typeof v === 'number')
  const avgCpu = cpuVals.length
    ? Math.round(cpuVals.reduce((a, b) => a + b, 0) / cpuVals.length)
    : null

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <MetricCard
          label="CPU (servers)"
          value={avgCpu == null ? '—' : `${avgCpu}%`}
          detail="From Control Plane server snapshots"
        />
        <MetricCard
          label="Servers reported"
          value={String(serverRows.length)}
          detail="GET /servers/{id}/metrics"
        />
        <MetricCard
          label="Series charts"
          value="API"
          detail="Use /servers/{id}/metrics/series for historical series"
        />
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Servers</CardTitle>
          <CardDescription>
            Live snapshots from Agent heartbeat / continuous metrics ingest. Empty fields mean the
            Agent has not reported that sample yet.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Server</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>RAM</TableHead>
                <TableHead>Disk</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {serverRows.map((server) => {
                const mem = pct(
                  server.metrics?.memoryUsedBytes,
                  server.metrics?.memoryTotalBytes,
                )
                const disk = pct(server.metrics?.diskUsedBytes, server.metrics?.diskTotalBytes)
                return (
                  <TableRow key={server.id}>
                    <TableCell>
                      <Link href={`/servers/${server.id}/metrics`} className="hover:underline">
                        {server.name}
                      </Link>
                    </TableCell>
                    <TableCell className="tabular">
                      {server.metrics?.cpuPercent == null
                        ? '—'
                        : `${Math.round(server.metrics.cpuPercent)}%`}
                    </TableCell>
                    <TableCell className="tabular">{mem == null ? '—' : `${mem}%`}</TableCell>
                    <TableCell className="tabular">{disk == null ? '—' : `${disk}%`}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {server.error ?? server.metrics?.source ?? '—'}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={server.status as Status} showDot />
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
