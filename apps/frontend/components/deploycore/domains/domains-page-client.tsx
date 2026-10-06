'use client'

import { useCallback, useEffect, useState } from 'react'
import { AddDomainDialog } from '@/components/deploycore/domains/add-domain-dialog'
import { DomainsFilterTable } from '@/components/deploycore/domains/domains-filter-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { apiClient } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import {
  loadProductionDomains,
  type DomainApplicationOption,
} from '@/lib/control-plane/domain-read'
import type { DomainRecord } from '@/lib/types'

export function DomainsPageClient() {
  const { activeOrg } = useOrganization()
  const [domains, setDomains] = useState<DomainRecord[] | null>(null)
  const [applications, setApplications] = useState<DomainApplicationOption[]>([])
  const [loadedOrgId, setLoadedOrgId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [errorOrgId, setErrorOrgId] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((key) => key + 1)
  }, [])

  useEffect(() => {
    const organizationId = activeOrg?.id
    if (!organizationId) return
    let cancelled = false

    async function loadDomains(orgId: string) {
      try {
        const loaded = await loadProductionDomains(apiClient, orgId)
        if (cancelled) return
        setDomains(loaded.domains)
        setApplications(loaded.applications)
        setLoadedOrgId(orgId)
        setError(null)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Unable to load domains from control plane')
        setErrorOrgId(orgId)
      }
    }

    void loadDomains(organizationId)
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, refreshKey])

  const organizationId = activeOrg?.id ?? null
  const domainsReady = organizationId !== null && loadedOrgId === organizationId && domains !== null
  const loadError = organizationId !== null && errorOrgId === organizationId ? error : null
  const selectableApplications = domainsReady ? applications : []

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Domains"
        description="Custom domains, DNS verification, and TLS certificate lifecycle across every application."
        actions={
          organizationId ? (
            <AddDomainDialog applications={selectableApplications} onSuccess={reload} />
          ) : null
        }
      />
      {!organizationId ? (
        <ErrorState title="Could not load domains" message="Select an organization to load domains." />
      ) : loadError ? (
        <ErrorState title="Failed to load domains" message={loadError} onRetry={reload} />
      ) : !domainsReady || !domains ? (
        <LoadingState label="Loading domains..." />
      ) : (
        <DomainsFilterTable domains={domains} />
      )}
    </PageContainer>
  )
}
