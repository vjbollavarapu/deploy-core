'use client'

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import { PageContainer } from '@/components/platform/page-container'
import { ProviderBadge } from '@/components/platform/provider-badge'
import { ResourceHeader } from '@/components/platform/resource-header'
import { ServerHeaderActions } from '@/components/deploycore/servers/server-header-actions'
import { ServerSubnav } from '@/components/platform/server-subnav'
import { StatusBadge } from '@/components/platform/status-badge'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { EmptyState } from '@/components/platform/empty-state'
import { Server as ServerIcon } from 'lucide-react'
import { apiClient, ApiError, type Server as WireServer } from '@/lib/api'
import { useApiQuery } from '@/hooks/use-api-query'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import {
  displayServerValue,
  findServer,
  wireServerToViewModel,
} from '@/lib/servers'
import type { Server } from '@/lib/types'

type GetServerResponse = { server?: WireServer }

interface ServerDetailContextValue {
  serverId: string
  server: Server
  reload: () => void
}

const ServerDetailContext = createContext<ServerDetailContextValue | null>(null)

export function useServerDetail(): ServerDetailContextValue {
  const ctx = useContext(ServerDetailContext)
  if (!ctx) {
    throw new Error('useServerDetail must be used within ServerDetailShell')
  }
  return ctx
}

interface ServerDetailShellProps {
  serverId: string
  children: ReactNode
}

export function ServerDetailShell({ serverId, children }: ServerDetailShellProps) {
  const demo = isDemoModeEnabled()

  const fetcher = useCallback(async (): Promise<Server> => {
    if (demo) {
      const local = findServer(serverId)
      if (!local) {
        throw new ApiError(404, 'Server not found', 'RESOURCE_NOT_FOUND')
      }
      return local
    }

    const res = await apiClient.get<GetServerResponse>(`/servers/${serverId}`)
    if (!res.server?.id) {
      throw new ApiError(404, 'Server not found', 'RESOURCE_NOT_FOUND')
    }
    return wireServerToViewModel(res.server)
  }, [demo, serverId])

  const { data: server, isLoading, error, errorCode, reload } = useApiQuery(fetcher)

  const value = useMemo(() => {
    if (!server) return null
    return { serverId, server, reload }
  }, [serverId, server, reload])

  if (isLoading && !server) {
    return (
      <PageContainer density="wide">
        <LoadingState variant="page" label="Loading server…" />
      </PageContainer>
    )
  }

  if (errorCode === 'RESOURCE_NOT_FOUND' || (!server && errorCode === 'RESOURCE_NOT_FOUND')) {
    return (
      <PageContainer density="wide">
        <EmptyState
          icon={ServerIcon}
          title="Server not found"
          description="This server does not exist or you do not have access to it."
        />
      </PageContainer>
    )
  }

  if (error || !server || !value) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load server"
          message={error || 'Unable to load server from the Control Plane'}
          onRetry={reload}
        />
      </PageContainer>
    )
  }

  const regionLabel = displayServerValue(server.region)
  const ipLabel = displayServerValue(server.ip)
  const agentLabel = displayServerValue(server.agentVersion)
  const heartbeatLabel = server.lastHeartbeat
    ? `Heartbeat ${server.lastHeartbeat}`
    : 'No heartbeat yet'
  const containersLabel =
    server.containers != null ? `${server.containers} containers` : 'Containers —'

  return (
    <ServerDetailContext.Provider value={value}>
      <PageContainer density="wide">
        <ResourceHeader
          title={server.name}
          description={`${displayServerValue(server.provider)} · ${regionLabel}`}
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
              <span className="font-mono">{ipLabel}</span>
              <span className="font-mono">{agentLabel}</span>
              <span>{heartbeatLabel}</span>
              <span>{containersLabel}</span>
            </div>
          }
          actions={<ServerHeaderActions server={server} />}
        />
        <ServerSubnav serverId={serverId} />
        {children}
      </PageContainer>
    </ServerDetailContext.Provider>
  )
}
