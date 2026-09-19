import Link from 'next/link'
import { ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { getRecentDeployments } from '@/lib/dashboard'

export function RecentDeploymentsCard() {
  const deployments = getRecentDeployments(8)

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle>Recent Deployments</CardTitle>
        <CardAction>
          <Button variant="ghost" size="sm" nativeButton={false} render={<Link href="/deployments" />}>
            View all
            <ArrowRight data-icon="inline-end" />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="p-0">
        <ul className="divide-y divide-border">
          {deployments.map((dep) => (
            <li key={dep.id}>
              <Link
                href={`/deployments/${dep.id}`}
                className="flex items-center gap-3 px-4 py-2.5 hover:bg-muted/40"
              >
                <StatusBadge status={dep.status} className="shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{dep.application}</span>
                    <span className="shrink-0 font-mono text-xs text-muted-foreground">
                      #{dep.number}
                    </span>
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{dep.commitMessage}</p>
                </div>
                <EnvironmentBadge
                  environment={dep.environment}
                  className="hidden shrink-0 md:inline-flex"
                />
                <span className="shrink-0 text-xs text-muted-foreground tabular">{dep.startedAt}</span>
              </Link>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
