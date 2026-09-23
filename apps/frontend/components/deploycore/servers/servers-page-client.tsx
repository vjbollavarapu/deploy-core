'use client'

import { useCallback, useEffect, useState } from 'react'
import { AddServerWizard } from '@/components/deploycore/servers/add-server-wizard'
import { ServersFilterTable } from '@/components/deploycore/servers/servers-filter-table'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { apiClient, type Page, type Server as WireServer } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { wireServerToViewModel } from '@/lib/servers'
import type { Server } from '@/lib/types'

interface ServersPageClientProps {
  servers: Server[]
}

export function ServersPageClient({ servers: fallbackServers }: ServersPageClientProps) {
  const { activeOrg } = useOrganization()
  const [serverList, setServerList] = useState<Server[]>(fallbackServers)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadServers() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const res = await apiClient.get<Page<WireServer>>(`/servers?organizationId=${activeOrg.id}`)

        if (cancelled) return

        if (Array.isArray(res?.items)) {
          const fallbackMap = new Map(fallbackServers.map((s) => [s.name.toLowerCase(), s]))
          const mapped: Server[] = res.items.map((wire) => {
            const fb = wire.name ? fallbackMap.get(wire.name.toLowerCase()) : undefined
            return wireServerToViewModel(wire, fb)
          })

          if (!cancelled) {
            setServerList(mapped)
          }
        }
      } catch (err) {
        if (cancelled) return
        setError(
          err instanceof Error ? err.message : 'Unable to load servers from control plane',
        )
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadServers()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackServers, refreshKey])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Servers"
        description="Physical and virtual hosts running your containers."
        actions={<AddServerWizard onSuccess={reload} />}
      />

      {error ? (
        <ErrorState
          title="Could not load servers"
          message={error}
          onRetry={reload}
        />
      ) : isLoading && serverList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading servers…" />
      ) : (
        <ServersFilterTable servers={serverList} />
      )}
    </PageContainer>
  )
}
