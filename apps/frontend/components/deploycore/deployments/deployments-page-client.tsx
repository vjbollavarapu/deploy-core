'use client'

import { useCallback, useEffect, useState } from 'react'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { DeploymentsFilterTable } from '@/components/deploycore/deployments/deployments-filter-table'
import {
  apiClient,
  type Page,
  type Application as WireApplication,
  type Deployment as WireDeployment,
} from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import {
  buildDeploymentEvents,
  buildDeploymentSteps,
} from '@/lib/deployments'
import type {
  Deployment,
  DeploymentFailureReason,
  DeploymentPhase,
  Status,
} from '@/lib/types'

interface DeploymentsPageClientProps {
  deployments: Deployment[]
}

function mapWireDeploymentStatus(rawStatus?: string): {
  status: Status
  phase: DeploymentPhase
  failureReason?: DeploymentFailureReason
} {
  switch (rawStatus) {
    case 'PENDING':
      return { status: 'pending', phase: 'PENDING' }
    case 'QUEUED':
      return { status: 'queued', phase: 'QUEUED' }
    case 'PREPARING':
      return { status: 'deploying', phase: 'PREPARING' }
    case 'FETCHING_SOURCE':
      return { status: 'deploying', phase: 'FETCHING_SOURCE' }
    case 'BUILDING':
      return { status: 'deploying', phase: 'BUILDING' }
    case 'IMAGE_READY':
      return { status: 'deploying', phase: 'IMAGE_READY' }
    case 'CREATING_CONTAINER':
      return { status: 'deploying', phase: 'CREATING_CONTAINER' }
    case 'STARTING':
      return { status: 'deploying', phase: 'STARTING' }
    case 'HEALTH_CHECKING':
      return { status: 'deploying', phase: 'HEALTH_CHECKING' }
    case 'ACTIVATING':
      return { status: 'deploying', phase: 'ACTIVATING' }
    case 'RUNNING':
      return { status: 'healthy', phase: 'RUNNING' }
    case 'SOURCE_FAILED':
      return { status: 'failed', phase: 'FETCHING_SOURCE', failureReason: 'SOURCE_FAILED' }
    case 'BUILD_FAILED':
      return { status: 'failed', phase: 'BUILDING', failureReason: 'BUILD_FAILED' }
    case 'IMAGE_FAILED':
      return { status: 'failed', phase: 'IMAGE_READY', failureReason: 'IMAGE_FAILED' }
    case 'CONTAINER_FAILED':
      return { status: 'failed', phase: 'CREATING_CONTAINER', failureReason: 'CONTAINER_FAILED' }
    case 'START_FAILED':
      return { status: 'failed', phase: 'STARTING', failureReason: 'START_FAILED' }
    case 'HEALTH_CHECK_FAILED':
      return { status: 'failed', phase: 'HEALTH_CHECKING', failureReason: 'HEALTH_CHECK_FAILED' }
    case 'ROUTING_FAILED':
      return { status: 'failed', phase: 'ACTIVATING', failureReason: 'ROUTING_FAILED' }
    case 'TIMEOUT':
      return { status: 'failed', phase: 'BUILDING', failureReason: 'TIMEOUT' }
    case 'CANCELLED':
      return { status: 'cancelled', phase: 'PREPARING', failureReason: 'CANCELLED' }
    default:
      return { status: 'healthy', phase: 'RUNNING' }
  }
}

export function DeploymentsPageClient({ deployments: fallbackDeployments }: DeploymentsPageClientProps) {
  const { activeOrg } = useOrganization()
  const [deploymentList, setDeploymentList] = useState<Deployment[]>(fallbackDeployments)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadDeployments() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const [depRes, appRes] = await Promise.all([
          apiClient.get<Page<WireDeployment>>(`/deployments?organizationId=${activeOrg.id}`),
          apiClient.get<Page<WireApplication>>(`/applications?organizationId=${activeOrg.id}`).catch(() => null),
        ])

        if (cancelled) return

        const appNamesById: Record<string, string> = {}
        if (Array.isArray(appRes?.items)) {
          for (const app of appRes.items) {
            if (app.id && app.name) {
              appNamesById[app.id] = app.name
            }
          }
        }

        if (Array.isArray(depRes?.items)) {
          const mapped: Deployment[] = depRes.items.map((item, index) => {
            const { status, phase, failureReason } = mapWireDeploymentStatus(item.status)
            const appName = (item.applicationId && appNamesById[item.applicationId]) || 'Core Platform Service'

            return {
              id: item.id || `dep-${index}`,
              number: depRes.items.length - index,
              applicationId: item.applicationId || '',
              application: appName,
              project: 'Core Platform',
              environment: 'production',
              revision: item.revisionId ? item.revisionId.slice(0, 8) : `rev-${depRes.items.length - index}`,
              commit: item.id ? item.id.slice(0, 7) : 'a1b2c3d',
              commitMessage: 'Deploy workload updates',
              author: { name: 'Operator' },
              triggeredBy: item.trigger === 'manual' ? 'Manual Trigger' : item.trigger === 'git_push' ? 'Git Push' : (item.trigger || 'Manual Trigger'),
              status,
              phase,
              failureReason,
              duration: '38s',
              startedAt: item.createdAt ? new Date(item.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : 'Just now',
              repo: 'github.com/deploycore/service',
              branch: 'main',
              server: 'srv-primary',
              image: 'ghcr.io/deploycore/app:latest',
              steps: buildDeploymentSteps(status, failureReason),
              events: buildDeploymentEvents(status, appName, failureReason),
            }
          })

          if (!cancelled) {
            setDeploymentList(mapped)
          }
        } else {
          if (!cancelled) {
            setDeploymentList([])
          }
        }
      } catch (err) {
        if (cancelled) return
        setError(
          err instanceof Error ? err.message : 'Unable to load deployments from control plane',
        )
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadDeployments()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackDeployments, refreshKey])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Deployments"
        description="Every deployment across all projects and environments, most recent first."
      />

      {error ? (
        <ErrorState
          title="Could not load deployments"
          message={error}
          onRetry={reload}
        />
      ) : isLoading && deploymentList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading deployments…" />
      ) : (
        <DeploymentsFilterTable deployments={deploymentList} />
      )}
    </PageContainer>
  )
}
