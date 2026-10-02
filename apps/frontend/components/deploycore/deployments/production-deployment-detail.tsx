'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient } from '@/lib/api'
import { loadProductionDeployment, type DeploymentDetail } from '@/lib/control-plane/detail-read'
import type { Status } from '@/lib/types'

function show(value: string | null | undefined): string {
  return value && value.trim() ? value : '—'
}

function badgeStatus(status: string): Status {
  if (status === 'RUNNING') return 'healthy'
  if (status === 'CANCELLED') return 'cancelled'
  if (status === 'PENDING') return 'pending'
  if (status === 'QUEUED') return 'queued'
  if (status.endsWith('_FAILED') || status === 'TIMEOUT') return 'failed'
  if (
    status === 'PREPARING' ||
    status === 'FETCHING_SOURCE' ||
    status === 'BUILDING' ||
    status === 'IMAGE_READY' ||
    status === 'CREATING_CONTAINER' ||
    status === 'STARTING' ||
    status === 'HEALTH_CHECKING' ||
    status === 'ACTIVATING'
  ) {
    return 'deploying'
  }
  return 'unknown'
}

export function ProductionDeploymentDetail({ deploymentId }: { deploymentId: string }) {
  const [deployment, setDeployment] = useState<DeploymentDetail | null>(null)
  const [phase, setPhase] = useState<'loading' | 'ready' | 'missing' | 'error'>('loading')
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    void loadProductionDeployment(apiClient, deploymentId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setDeployment(result.value)
        setPhase('ready')
        return
      }
      setDeployment(null)
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
  }, [deploymentId, attempt])

  if (phase === 'loading') {
    return (
      <PageContainer density="wide">
        <LoadingState label="Loading deployment…" />
      </PageContainer>
    )
  }
  if (phase === 'missing') {
    return (
      <PageContainer density="wide">
        <ErrorState title="Deployment not found" message={`No deployment exists for ${deploymentId}.`} />
      </PageContainer>
    )
  }
  if (phase === 'error' || !deployment) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load deployment"
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

  const applicationLabel = show(deployment.applicationName ?? deployment.applicationId)

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title="Deployment"
        description={deployment.id}
        breadcrumbs={[
          { label: 'Deployments', href: '/deployments' },
          { label: deployment.id.slice(0, 8) },
        ]}
        badges={<StatusBadge status={badgeStatus(deployment.status)} />}
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {deployment.applicationId ? (
              <Link href={`/applications/${deployment.applicationId}`} className="hover:text-foreground hover:underline">
                {applicationLabel}
              </Link>
            ) : (
              <span>{applicationLabel}</span>
            )}
            <span>{show(deployment.environmentName ?? deployment.environmentId)}</span>
            <span className="font-mono">{deployment.status}</span>
          </div>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Details</CardTitle>
            <CardDescription>Fields returned by the control plane for this deployment.</CardDescription>
          </CardHeader>
          <CardContent>
            <DetailList
              columns={2}
              items={[
                { label: 'Deployment ID', value: <span className="font-mono text-xs">{deployment.id}</span> },
                { label: 'Application', value: applicationLabel },
                { label: 'Environment', value: show(deployment.environmentName ?? deployment.environmentId) },
                { label: 'Server', value: show(deployment.serverName ?? deployment.serverId) },
                { label: 'Status', value: deployment.status },
                { label: 'Trigger', value: show(deployment.trigger) },
                { label: 'Error code', value: show(deployment.errorCode) },
                { label: 'Error message', value: show(deployment.errorMessage) },
                { label: 'Target revision', value: <span className="font-mono text-xs">{show(deployment.targetRevisionId)}</span> },
                { label: 'Active revision', value: <span className="font-mono text-xs">{show(deployment.activeRevisionId)}</span> },
                { label: 'Started', value: show(deployment.startedAt) },
                { label: 'Finished', value: show(deployment.finishedAt) },
                { label: 'Duration', value: show(deployment.duration) },
              ]}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Events</CardTitle>
            <CardDescription>
              {deployment.events.length === 0 ? 'No events were returned.' : `${deployment.events.length} events`}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {deployment.events.length === 0 ? (
              <p className="text-sm text-muted-foreground">This deployment has no recorded events.</p>
            ) : (
              <ol className="flex flex-col gap-3">
                {deployment.events.map((event) => (
                  <li key={event.id} className="flex flex-col gap-0.5">
                    <span className="font-mono text-[11px] text-muted-foreground">{show(event.createdAt)}</span>
                    <span className="text-xs font-medium text-foreground">{event.toStatus}</span>
                    <span className="text-sm text-muted-foreground">{event.message || '—'}</span>
                  </li>
                ))}
              </ol>
            )}
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  )
}
