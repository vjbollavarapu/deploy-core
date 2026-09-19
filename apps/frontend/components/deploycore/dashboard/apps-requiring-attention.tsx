import Link from 'next/link'
import { AlertTriangle, ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/platform/empty-state'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { getApplicationsRequiringAttention } from '@/lib/dashboard'

export function AppsRequiringAttention() {
  const apps = getApplicationsRequiringAttention()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle>Applications Requiring Attention</CardTitle>
        <CardAction>
          <Button variant="ghost" size="sm" nativeButton={false} render={<Link href="/applications" />}>
            View all
            <ArrowRight data-icon="inline-end" />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className={apps.length === 0 ? 'py-4' : 'p-0'}>
        {apps.length === 0 ? (
          <EmptyState
            icon={AlertTriangle}
            title="All applications healthy"
            description="No failed, degraded, or stopped applications right now."
            className="border-0 py-2"
          />
        ) : (
          <ul className="divide-y divide-border">
            {apps.map((app) => {
              const cpuPct = Math.round((app.cpu / app.cpuLimit) * 100)
              const memPct = Math.round((app.memory / app.memoryLimit) * 100)
              return (
                <li key={app.id}>
                  <Link
                    href={`/applications/${app.id}`}
                    className="flex flex-col gap-2 px-4 py-2.5 hover:bg-muted/40 sm:flex-row sm:items-center"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-medium">{app.name}</span>
                        <StatusBadge status={app.status} />
                        <EnvironmentBadge environment={app.environment} />
                      </div>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">
                        {app.project} · {app.server} · {app.revision}
                      </p>
                    </div>
                    <div className="grid w-full gap-1.5 sm:w-48">
                      <ResourceUsageBar label="CPU" value={cpuPct} detail={`${cpuPct}%`} size="sm" />
                      <ResourceUsageBar label="Mem" value={memPct} detail={`${memPct}%`} size="sm" />
                    </div>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
