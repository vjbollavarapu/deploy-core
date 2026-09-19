import { Boxes, Database, Globe, KeyRound, Rocket } from 'lucide-react'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import type { Application, Project, Status } from '@/lib/types'
import Link from 'next/link'
import { environmentSlug } from '@/lib/projects'

interface ProjectOverviewProps {
  project: Project
  applicationCount: number
  databaseCount: number
  domainCount: number
  secretCount: number
  environmentHealth: Record<string, Status>
  recentApplications: Application[]
}

export function ProjectOverview({
  project,
  applicationCount,
  databaseCount,
  domainCount,
  secretCount,
  environmentHealth,
  recentApplications,
}: ProjectOverviewProps) {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <MetricCard label="Applications" value={String(applicationCount)} icon={Rocket} />
        <MetricCard label="Environments" value={String(project.environments.length)} icon={Boxes} />
        <MetricCard label="Databases" value={String(databaseCount)} icon={Database} />
        <MetricCard label="Domains" value={String(domainCount)} icon={Globe} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Environment health</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {project.environments.map((env) => (
              <Link
                key={env}
                href={`/projects/${project.slug}/environments/${environmentSlug(env)}`}
                className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 hover:bg-muted/40"
              >
                <EnvironmentBadge environment={env} />
                <StatusBadge status={environmentHealth[env] ?? 'unknown'} />
              </Link>
            ))}
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Project summary</CardTitle>
          </CardHeader>
          <CardContent>
            <DetailList
              items={[
                { label: 'Owner', value: project.owner.name },
                { label: 'Slug', value: project.slug },
                { label: 'Latest deployment', value: project.lastDeployment },
                { label: 'Updated', value: project.updatedAt },
                { label: 'Secrets referenced', value: String(secretCount) },
                {
                  label: 'Health',
                  value: <StatusBadge status={project.health} />,
                },
              ]}
            />
          </CardContent>
        </Card>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="size-3.5 text-muted-foreground" aria-hidden />
            Recent applications
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <ul className="divide-y divide-border">
            {recentApplications.slice(0, 5).map((app) => (
              <li key={app.id}>
                <Link
                  href={`/applications/${app.id}`}
                  className="flex items-center justify-between gap-3 px-4 py-2.5 hover:bg-muted/40"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{app.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {app.environment} · {app.runtime}
                    </p>
                  </div>
                  <StatusBadge status={app.status} />
                </Link>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
