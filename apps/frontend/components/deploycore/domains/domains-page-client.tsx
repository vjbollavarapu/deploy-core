'use client'

import { useCallback } from 'react'
import { AddDomainDialog } from '@/components/deploycore/domains/add-domain-dialog'
import { DomainsFilterTable } from '@/components/deploycore/domains/domains-filter-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { useApiQuery } from '@/hooks/use-api-query'
import { apiClient, type Domain, type Application } from '@/lib/api'
import type { DomainRecord } from '@/lib/types'

export function DomainsPageClient() {
  const fetchDomains = useCallback(async () => {
    const [domainsRes, appsRes] = await Promise.all([
      apiClient.get<{ domains: Domain[] }>('/domains'),
      apiClient.get<{ items: Application[] }>('/applications')
    ])
    
    const appsMap = new Map(appsRes.items?.map(app => [app.id, app]) || [])
    
    return domainsRes.domains.map(d => {
       const app = appsMap.get(d.applicationId)
       return {
          id: d.id,
          domain: d.hostname,
          applicationId: d.applicationId,
          application: app?.name || 'Unknown',
          environment: 'production', // Mapped statically for now to avoid N+1
          routingPort: d.internalPort,
          dnsVerified: d.dnsStatus === 'VALID',
          https: d.forceHttps,
          certExpiry: 'N/A',
          certExpiryDays: 0,
          status: (d.tlsStatus === 'ACTIVE' && d.dnsStatus === 'VALID') ? 'healthy' : 'pending',
          tlsState: d.tlsStatus,
          certificateIssuer: null,
          certificateIssuedAt: null,
          nextRenewalAt: null,
          lastValidatedAt: null,
          validationMessage: '',
          primary: d.isPrimary,
          forceHttps: d.forceHttps,
          requiredRecord: { type: 'CNAME', name: d.hostname, value: 'proxy.deploycore.io' },
          detectedRecord: null,
          redirectRules: []
       } as DomainRecord
    })
  }, [])

  const { data: domains, isLoading, error, reload } = useApiQuery(fetchDomains)

  if (isLoading) return <LoadingState label="Loading domains..." />
  if (error) return <ErrorState title="Failed to load domains" message={error} onRetry={reload} />

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Domains"
        description="Custom domains, DNS verification, and TLS certificate lifecycle across every application."
        actions={<AddDomainDialog onSuccess={reload} />}
      />
      <DomainsFilterTable domains={domains || []} />
    </PageContainer>
  )
}
