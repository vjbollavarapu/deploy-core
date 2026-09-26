'use client'

import { useState } from 'react'
import { RefreshCw, Shield } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { ApplicationsTable } from '@/components/deploycore/applications/applications-table'
import { CodeBlock } from '@/components/platform/code-block'
import { CopyButton } from '@/components/platform/copy-button'
import { DetailList } from '@/components/platform/detail-list'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { ServerMetricsChart } from '@/components/deploycore/servers/server-metrics-chart'
import { apiClient, ApiError } from '@/lib/api'
import {
  buildRegistrationCommand,
  canIssueInitialRegistrationToken,
  displayServerValue,
  getServerApplications,
  getServerContainers,
  serverMetricSeries,
  UNAVAILABLE,
} from '@/lib/servers'
import { cn } from '@/lib/utils'
import type { Server } from '@/lib/types'

interface ServerOverviewProps {
  server: Server
}

function ResourceOrUnavailable({
  label,
  value,
  detail,
}: {
  label: string
  value: number | null
  detail: string
}) {
  if (value == null) {
    return (
      <div className="flex flex-col gap-1">
        <div className="flex items-center justify-between gap-2 text-xs">
          <span className="text-muted-foreground">{label}</span>
          <span className="text-muted-foreground">{detail}</span>
        </div>
        <p className="text-sm text-muted-foreground">Not reported</p>
      </div>
    )
  }
  return <ResourceUsageBar label={label} value={value} detail={detail} />
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
            <ResourceOrUnavailable
              label="CPU"
              value={server.cpu}
              detail={
                server.cpuCores != null ? `${server.cpuCores} cores` : UNAVAILABLE
              }
            />
            <ResourceOrUnavailable
              label="Memory"
              value={server.memory}
              detail={
                server.memoryTotalGb != null
                  ? `${server.memoryTotalGb} GB total`
                  : UNAVAILABLE
              }
            />
            <ResourceOrUnavailable
              label="Disk"
              value={server.disk}
              detail={
                server.diskTotalGb != null ? `${server.diskTotalGb} GB total` : UNAVAILABLE
              }
            />
            <Separator />
            <div className="grid grid-cols-3 gap-4 text-sm">
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Load average</span>
                <span className="font-mono text-foreground">
                  {server.load ? server.load.join(' / ') : UNAVAILABLE}
                </span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Containers</span>
                <span className="text-foreground">
                  {containers.length > 0
                    ? `${containers.length} tracked`
                    : displayServerValue(server.containers)}
                </span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Uptime</span>
                <span className="text-foreground">{displayServerValue(server.uptime)}</span>
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
            <DetailRow label="Public IP" value={displayServerValue(server.ip)} copyable={Boolean(server.ip)} />
            <DetailRow
              label="Private IP"
              value={displayServerValue(server.privateIp)}
              copyable={Boolean(server.privateIp)}
            />
            <DetailRow label="OS" value={displayServerValue(server.os)} />
            <DetailRow label="Architecture" value={displayServerValue(server.arch)} />
            <DetailRow label="Docker" value={displayServerValue(server.dockerVersion)} />
            <DetailRow label="Agent" value={displayServerValue(server.agentVersion)} />
            <DetailRow
              label="Last heartbeat"
              value={server.lastHeartbeat ?? 'No heartbeat yet'}
            />
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Recent utilisation</CardTitle>
          <CardDescription>CPU and memory over the last 24 samples.</CardDescription>
        </CardHeader>
        <CardContent>
          {series.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No live utilisation samples are available for this host yet.
            </p>
          ) : (
            <ServerMetricsChart series={series} heightClassName="h-40" showDisk={false} />
          )}
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
      <div className="flex min-w-0 items-center gap-1.5">
        <span className="truncate font-mono text-foreground">{value}</span>
        {copyable && value !== UNAVAILABLE ? <CopyButton value={value} /> : null}
      </div>
    </div>
  )
}

type RegistrationTokenResponse = {
  registrationToken?: {
    serverId?: string
    agentId?: string
    token?: string
    expiresAt?: string
  }
}

function formatTokenExpiry(expiresAt: string | null | undefined): string | null {
  if (!expiresAt) return null
  const d = new Date(expiresAt)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export function ServerAgentPanel({ server }: { server: Server }) {
  const canRecover = canIssueInitialRegistrationToken(server)
  const [issuing, setIssuing] = useState(false)
  const [token, setToken] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<string | null>(null)

  const expiryLabel = formatTokenExpiry(expiresAt)
  const command =
    token != null ? buildRegistrationCommand(token, server.id) : null

  async function issueRegistrationToken() {
    if (issuing || !canRecover) return
    setIssuing(true)
    try {
      const res = await apiClient.post<RegistrationTokenResponse>(
        `/servers/${server.id}/registration-token`,
      )
      const next = res.registrationToken?.token
      if (!next) {
        toast.error('Control Plane did not return a registration token.')
        return
      }
      setToken(next)
      setExpiresAt(res.registrationToken?.expiresAt ?? null)
      toast.success('Registration token issued')
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Failed to issue registration token')
    } finally {
      setIssuing(false)
    }
  }

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
              { label: 'Version', value: displayServerValue(server.agentVersion) },
              { label: 'Status', value: <StatusBadge status={server.status} /> },
              {
                label: 'Last heartbeat',
                value: server.lastHeartbeat ?? 'No heartbeat yet',
              },
              { label: 'Docker', value: displayServerValue(server.dockerVersion) },
              { label: 'OS', value: displayServerValue(server.os) },
              { label: 'Arch', value: displayServerValue(server.arch) },
            ]}
          />
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Registration</CardTitle>
          <CardDescription>
            {canRecover
              ? 'This server has never received an agent heartbeat. Issue a temporary registration token for first-time Agent install on this host.'
              : server.lastHeartbeatAt
                ? 'This server has previously connected. Use durable agent credentials after registration; do not rotate registration tokens for a healthy Agent identity from this panel.'
                : 'Agent registration status for this host.'}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex min-w-0 flex-col gap-3">
          <CodeBlock
            code={`deploycore-agent status\nsystemctl status deploycore-agent`}
            label="Diagnostics"
            className="min-w-0 w-full"
          />

          {canRecover ? (
            <div className="flex min-w-0 flex-col gap-3">
              <p className="text-xs text-muted-foreground">
                Tokens are temporary and single-use. Generating a new token supersedes any previous
                unused token for this server. Do not delete or recreate the server record.
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                disabled={issuing}
                onClick={() => void issueRegistrationToken()}
              >
                <RefreshCw
                  data-icon="inline-start"
                  className={cn(issuing && 'animate-spin')}
                />
                {issuing ? 'Generating…' : token ? 'Generate new token' : 'Generate registration token'}
              </Button>

              {token ? (
                <div className="min-w-0 rounded-lg border border-border bg-muted/40 px-3 py-2">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Shield className="size-3.5 shrink-0" />
                    Temporary token
                  </div>
                  <p className="mt-2 break-all font-mono text-sm text-foreground">{token}</p>
                  {expiryLabel ? (
                    <p className="mt-1.5 text-xs text-muted-foreground">Expires {expiryLabel}</p>
                  ) : null}
                </div>
              ) : null}

              {command ? (
                <CodeBlock
                  code={command}
                  label="Registration command"
                  className="min-w-0 w-full"
                />
              ) : null}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              First-install token recovery is available only when the Control Plane has never
              recorded an agent heartbeat for this server (offline, not yet registered).
            </p>
          )}
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
            { label: 'Provider', value: displayServerValue(server.provider) },
            { label: 'Region', value: displayServerValue(server.region) },
            {
              label: 'Public IP',
              value: (
                <span className="font-mono text-xs">{displayServerValue(server.ip)}</span>
              ),
            },
          ]}
        />
      </CardContent>
    </Card>
  )
}
