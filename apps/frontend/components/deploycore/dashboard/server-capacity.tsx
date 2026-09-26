import Link from 'next/link'
import { ArrowRight, Server } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/platform/empty-state'
import { ProviderBadge } from '@/components/platform/provider-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import { getServerCapacity } from '@/lib/dashboard'
import { displayServerValue, UNAVAILABLE } from '@/lib/servers'

export function ServerCapacity() {
  const servers = getServerCapacity()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle>Server Capacity</CardTitle>
        <CardAction>
          <Button variant="ghost" size="sm" nativeButton={false} render={<Link href="/servers" />}>
            View all
            <ArrowRight data-icon="inline-end" />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className={servers.length === 0 ? 'py-4' : 'p-0'}>
        {servers.length === 0 ? (
          <EmptyState
            icon={Server}
            title="No servers"
            description="Register your first server to monitor capacity."
            className="border-0 py-2"
          />
        ) : (
          <ul className="divide-y divide-border">
            {servers.map((server) => (
              <li key={server.id}>
                <Link
                  href={`/servers/${server.id}`}
                  className="grid gap-2 px-4 py-2.5 hover:bg-muted/40 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)] sm:items-center"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-sm font-medium">{server.name}</span>
                      <StatusBadge status={server.status} />
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2">
                      <ProviderBadge provider={server.provider} />
                      <span className="text-xs text-muted-foreground">
                        {displayServerValue(server.region)}
                      </span>
                      <span className="text-xs text-muted-foreground tabular">
                        agent {displayServerValue(server.agentVersion)}
                      </span>
                    </div>
                  </div>
                  <div className="grid gap-1.5">
                    {server.cpu != null ? (
                      <ResourceUsageBar
                        label="CPU"
                        value={server.cpu}
                        detail={`${server.cpu}%`}
                        size="sm"
                      />
                    ) : (
                      <p className="text-xs text-muted-foreground">CPU {UNAVAILABLE}</p>
                    )}
                    {server.memory != null ? (
                      <ResourceUsageBar
                        label="RAM"
                        value={server.memory}
                        detail={`${server.memory}%`}
                        size="sm"
                      />
                    ) : (
                      <p className="text-xs text-muted-foreground">RAM {UNAVAILABLE}</p>
                    )}
                    {server.disk != null ? (
                      <ResourceUsageBar
                        label="Disk"
                        value={server.disk}
                        detail={`${server.disk}%`}
                        size="sm"
                      />
                    ) : (
                      <p className="text-xs text-muted-foreground">Disk {UNAVAILABLE}</p>
                    )}
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
