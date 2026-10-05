'use client'

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import Link from 'next/link'
import { GitBranch } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ApplicationRedeployButton } from '@/components/deploycore/applications/application-redeploy-button'
import { ApplicationSubnav } from '@/components/platform/application-subnav'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { loadProductionApplication, type ApplicationDetail } from '@/lib/control-plane/detail-read'
import type { Status } from '@/lib/types'

const ApplicationDetailContext = createContext<ApplicationDetail | null>(null)
const ReloadApplicationContext = createContext<(() => Promise<ApplicationDetail | null>) | null>(null)

export function useProductionApplication(): ApplicationDetail | null {
  return useContext(ApplicationDetailContext)
}

export function useReloadProductionApplication(): (() => Promise<ApplicationDetail | null>) | null {
  return useContext(ReloadApplicationContext)
}

function show(value: string | null | undefined): string {
  return value && value.trim() ? value : '—'
}

export function ProductionApplicationShell({
  applicationId,
  children,
}: {
  applicationId: string
  children: ReactNode
}) {
  const [application, setApplication] = useState<ApplicationDetail | null>(null)
  const [phase, setPhase] = useState<'loading' | 'ready' | 'missing' | 'error'>('loading')
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const { activeOrg } = useOrganization()
  const applicationIdRef = useRef(applicationId)
  const organizationIdRef = useRef(activeOrg?.id ?? '')

  const reload = useCallback(async () => {
    const requestedApplicationId = applicationIdRef.current
    const requestedOrganizationId = organizationIdRef.current
    const result = await loadProductionApplication(apiClient, requestedApplicationId)
    if (
      applicationIdRef.current !== requestedApplicationId ||
      organizationIdRef.current !== requestedOrganizationId ||
      result.kind !== 'ok'
    ) {
      return null
    }
    if (requestedOrganizationId && result.value.organizationId && result.value.organizationId !== requestedOrganizationId) {
      return null
    }
    setApplication(result.value)
    setError(null)
    setPhase('ready')
    return result.value
  }, [])

  useEffect(() => {
    applicationIdRef.current = applicationId
    organizationIdRef.current = activeOrg?.id ?? ''
  }, [activeOrg?.id, applicationId])

  useEffect(() => {
    let cancelled = false
    void loadProductionApplication(apiClient, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setApplication(result.value)
        setPhase('ready')
        return
      }
      setApplication(null)
      if (result.kind === 'not-found') {
        setPhase('missing')
        return
      }
      setError(result.message)
      setPhase('error')
    })
    return () => {
      cancelled = true
    }
  }, [applicationId, attempt])

  if (phase === 'loading') {
    return (
      <PageContainer density="wide">
        <LoadingState label="Loading application…" />
      </PageContainer>
    )
  }

  if (phase === 'missing') {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Application not found"
          message={`No application exists for ${applicationId}.`}
        />
      </PageContainer>
    )
  }

  if (phase === 'error' || !application) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load application"
          message={error ?? 'Unable to reach the control plane'}
          onRetry={() => {
            setPhase('loading')
            setError(null)
            setAttempt((value) => value + 1)
          }}
        />
      </PageContainer>
    )
  }

  const projectLabel = show(application.projectName ?? application.projectId)
  const environmentLabel = show(application.environmentName ?? application.environmentId)
  const serverLabel = show(application.serverName ?? application.serverId)

  return (
    <ApplicationDetailContext.Provider value={application}>
      <ReloadApplicationContext.Provider value={reload}>
      <PageContainer density="wide">
        <ResourceHeader
          title={application.name}
          description={`${application.type}${application.slug ? ` · ${application.slug}` : ''}`}
          breadcrumbs={[
            { label: 'Applications', href: '/applications' },
            { label: projectLabel },
            { label: application.name },
          ]}
          badges={
            <>
              <StatusBadge status={application.status as Status} />
              <span className="rounded-md border border-border bg-muted/50 px-2 py-0.5 text-xs text-muted-foreground">
                {environmentLabel}
              </span>
            </>
          }
          meta={
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span className="font-mono">{application.id}</span>
              <span>{serverLabel}</span>
            </div>
          }
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                nativeButton={false}
                render={<Link href={`/applications/${application.id}/deployments`} />}
              >
                <GitBranch data-icon="inline-start" />
                Deployments
              </Button>
              <ApplicationRedeployButton
                applicationId={application.id}
                applicationName={application.name}
                status={application.status}
              />
            </div>
          }
        />
        <ApplicationSubnav applicationId={applicationId} />
        {children}
      </PageContainer>
      </ReloadApplicationContext.Provider>
    </ApplicationDetailContext.Provider>
  )
}
