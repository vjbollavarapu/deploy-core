import type { ReactNode } from 'react'
import { notFound } from 'next/navigation'
import { PageContainer } from '@/components/platform/page-container'
import { ProviderBadge } from '@/components/platform/provider-badge'
import { ResourceHeader } from '@/components/platform/resource-header'
import { ServerHeaderActions } from '@/components/deploycore/servers/server-header-actions'
import { ServerSubnav } from '@/components/platform/server-subnav'
import { StatusBadge } from '@/components/platform/status-badge'
import { findServer } from '@/lib/servers'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)

export default async function ServerLayout({
  children,
  params,
}: {
  children: ReactNode
  params: Promise<{ serverId: string }>
}) {
  const { serverId } = await params
  const server = findServer(serverId, servers)
  if (!server) notFound()

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={server.name}
        description={`${server.provider} · ${server.region}`}
        breadcrumbs={[
          { label: 'Servers', href: '/servers' },
          { label: server.name },
        ]}
        badges={
          <>
            <StatusBadge status={server.status} />
            <ProviderBadge provider={server.provider} />
          </>
        }
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-mono">{server.ip}</span>
            <span className="font-mono">{server.agentVersion}</span>
            <span>Heartbeat {server.lastHeartbeat}</span>
            <span>{server.containers} containers</span>
          </div>
        }
        actions={<ServerHeaderActions server={server} />}
      />
      <ServerSubnav serverId={serverId} />
      {children}
    </PageContainer>
  )
}
