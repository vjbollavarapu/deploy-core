import Link from 'next/link'
import { ExternalLink } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ActivityTimeline } from '@/components/platform/activity-timeline'
import { DetailList } from '@/components/platform/detail-list'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { HealthIndicator } from '@/components/platform/health-indicator'
import { MetricCard } from '@/components/platform/metric-card'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { CopyButton } from '@/components/platform/copy-button'
import type {
  ActivityItem,
  Application,
  Deployment,
  DomainRecord,
  LogLine,
} from '@/lib/types'
import { projects } from '@/lib/mock-data'
import { STATUS_CONFIG } from '@/lib/status'

interface ApplicationOverviewProps {
  application: Application
  latestDeployment?: Deployment
  primaryDomain?: DomainRecord
  recentLogs: LogLine[]
  activity: ActivityItem[]
}

function projectHref(projectId: string, projectName: string) {
  const project = projects.find((p) => p.id === projectId || p.name === projectName)
  return project ? `/projects/${project.slug}` : '/projects'
}

export function ApplicationOverview({
  application,
  latestDeployment,
  primaryDomain,
  recentLogs,
  activity,
}: ApplicationOverviewProps) {
  const url = application.domain
    ? application.domain.startsWith('http')
      ? application.domain
      : `https://${application.domain}`
    : null

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
        <MetricCard label="Runtime status" value={STATUS_CONFIG[application.status].label} />
        <MetricCard label="Revision" value={application.revision} />
        <MetricCard label="Deployment age" value={application.lastDeployment} />
        <MetricCard label="Uptime" value={application.uptime} />
        <MetricCard label="Instances" value={String(application.instances)} />
        <MetricCard label="Server" value={application.server} />
      </div>

      <div className="grid gap-4 xl:grid-cols-3">
        <Card size="sm" className="xl:col-span-2">
          <CardHeader>
            <CardTitle>Overview</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-6 sm:grid-cols-2">
            <DetailList
              items={[
                {
                  label: 'Health',
                  value: <HealthIndicator status={application.status} />,
                },
                {
                  label: 'URL',
                  value: url ? (
                    <span className="inline-flex max-w-full items-center gap-1">
                      <a
                        href={url}
                        target="_blank"
                        rel="noreferrer"
                        className="truncate font-mono text-xs hover:underline"
                      >
                        {application.domain}
                      </a>
                      <ExternalLink className="size-3 shrink-0 text-muted-foreground" aria-hidden />
                      <CopyButton value={url} label="Copy URL" />
                    </span>
                  ) : (
                    '—'
                  ),
                },
                {
                  label: 'Project',
                  value: (
                    <Link
                      href={projectHref(application.projectId, application.project)}
                      className="hover:underline"
                    >
                      {application.project}
                    </Link>
                  ),
                },
                {
                  label: 'Environment',
                  value: <EnvironmentBadge environment={application.environment} />,
                },
                { label: 'Type', value: application.runtime },
                {
                  label: 'Server',
                  value: (
                    <Link href="/servers" className="font-mono text-xs hover:underline">
                      {application.server}
                    </Link>
                  ),
                },
              ]}
            />
            <DetailList
              items={[
                { label: 'Source', value: <span className="font-mono text-xs">{application.repo}</span> },
                { label: 'Branch', value: <span className="font-mono text-xs">{application.branch}</span> },
                {
                  label: 'Commit',
                  value: (
                    <span className="inline-flex items-center gap-1">
                      <span className="font-mono text-xs">{application.commit}</span>
                      <CopyButton value={application.commit} label="Copy commit" />
                    </span>
                  ),
                },
                { label: 'Commit message', value: application.commitMessage },
                { label: 'Revision', value: <span className="font-mono text-xs">{application.revision}</span> },
                { label: 'Last deployment', value: application.lastDeployment },
              ]}
            />
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Resources</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <ResourceUsageBar
              label="CPU"
              value={application.cpu}
              detail={`${application.cpu}% · limit ${application.cpuLimit} cores`}
            />
            <ResourceUsageBar
              label="RAM"
              value={application.memory}
              detail={`${application.memory}% · limit ${application.memoryLimit} MiB`}
            />
            <div className="border-t border-border pt-3">
              <p className="mb-2 text-xs text-muted-foreground">Domain / TLS</p>
              {primaryDomain ? (
                <DetailList
                  items={[
                    { label: 'Domain', value: <span className="font-mono text-xs">{primaryDomain.domain}</span> },
                    {
                      label: 'HTTPS',
                      value: primaryDomain.https ? 'Enabled' : 'Disabled',
                    },
                    {
                      label: 'Certificate',
                      value: primaryDomain.https
                        ? `expires ${primaryDomain.certExpiry} (${primaryDomain.certExpiryDays}d)`
                        : '—',
                    },
                    {
                      label: 'Status',
                      value: <StatusBadge status={primaryDomain.status} />,
                    },
                  ]}
                />
              ) : (
                <p className="text-sm text-muted-foreground">No domain attached.</p>
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card size="sm">
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Recent deployment</CardTitle>
            <Button
              size="sm"
              variant="ghost"
              nativeButton={false}
              render={<Link href={`/applications/${application.id}/deployments`} />}
            >
              View all
            </Button>
          </CardHeader>
          <CardContent>
            {latestDeployment ? (
              <div className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    href={`/deployments/${latestDeployment.id}`}
                    className="font-mono text-sm font-medium hover:underline"
                  >
                    #{latestDeployment.number}
                  </Link>
                  <StatusBadge status={latestDeployment.status} />
                  <EnvironmentBadge environment={latestDeployment.environment} />
                </div>
                <DetailList
                  items={[
                    { label: 'Revision', value: latestDeployment.revision },
                    { label: 'Commit', value: `${latestDeployment.commit} — ${latestDeployment.commitMessage}` },
                    { label: 'Triggered by', value: latestDeployment.triggeredBy },
                    { label: 'Started', value: latestDeployment.startedAt },
                    { label: 'Duration', value: latestDeployment.duration },
                  ]}
                />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">No deployments yet.</p>
            )}
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Recent logs</CardTitle>
            <Button
              size="sm"
              variant="ghost"
              nativeButton={false}
              render={<Link href={`/applications/${application.id}/logs`} />}
            >
              Open logs
            </Button>
          </CardHeader>
          <CardContent>
            <ul className="max-h-56 space-y-1 overflow-y-auto font-mono text-[11px] leading-relaxed">
              {recentLogs.slice(-12).map((line) => (
                <li key={line.id} className="flex gap-2 text-muted-foreground">
                  <span className="shrink-0 tabular">{line.timestamp}</span>
                  <span
                    className={
                      line.level === 'error'
                        ? 'text-critical'
                        : line.level === 'warn'
                          ? 'text-warning'
                          : 'text-foreground'
                    }
                  >
                    {line.level}
                  </span>
                  <span className="truncate text-foreground">{line.message}</span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Activity</CardTitle>
        </CardHeader>
        <CardContent>
          {activity.length === 0 ? (
            <p className="text-sm text-muted-foreground">No recent activity for this application.</p>
          ) : (
            <ActivityTimeline items={activity} />
          )}
        </CardContent>
      </Card>
    </div>
  )
}
