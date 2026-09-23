'use client'

import { useCallback, useEffect, useState } from 'react'
import { VolumesFilterTable } from '@/components/deploycore/volumes/volumes-filter-table'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { apiClient, type Page, type Server as WireServer } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { wireVolumeToViewModel, type WireVolume } from '@/lib/volumes'
import type { Volume } from '@/lib/types'

interface VolumesPageClientProps {
  volumes: Volume[]
}

export function VolumesPageClient({ volumes: fallbackVolumes }: VolumesPageClientProps) {
  const { activeOrg } = useOrganization()
  const [volumeList, setVolumeList] = useState<Volume[]>(fallbackVolumes)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadVolumes() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const [volRes, srvRes] = await Promise.all([
          apiClient.get<Page<WireVolume>>(`/volumes?organizationId=${activeOrg.id}`),
          apiClient.get<Page<WireServer>>(`/servers?organizationId=${activeOrg.id}`).catch(() => null),
        ])

        if (cancelled) return

        const serverNamesById: Record<string, string> = {}
        if (Array.isArray(srvRes?.items)) {
          for (const s of srvRes.items) {
            if (s.id && s.name) {
              serverNamesById[s.id] = s.name
            }
          }
        }

        if (Array.isArray(volRes?.items) && volRes.items.length > 0) {
          const fallbackMap = new Map(fallbackVolumes.map((v) => [v.name.toLowerCase(), v]))
          const mapped: Volume[] = volRes.items.map((wire) => {
            const fb = wire.name ? fallbackMap.get(wire.name.toLowerCase()) : undefined
            const serverName = wire.serverId ? serverNamesById[wire.serverId] : undefined
            return wireVolumeToViewModel(wire, fb, serverName)
          })

          if (!cancelled) {
            setVolumeList(mapped)
          }
        } else {
          if (!cancelled) {
            setVolumeList([])
          }
        }
      } catch (err) {
        if (cancelled) return
        setError(
          err instanceof Error ? err.message : 'Unable to load volumes from control plane',
        )
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadVolumes()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackVolumes, refreshKey])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Volumes"
        description="Persistent storage volumes attached to applications and databases. Deletes require confirmation."
      />

      {error ? (
        <ErrorState
          title="Could not load volumes"
          message={error}
          onRetry={reload}
        />
      ) : isLoading && volumeList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading volumes…" />
      ) : (
        <VolumesFilterTable volumes={volumeList} onDelete={reload} />
      )}
    </PageContainer>
  )
}
