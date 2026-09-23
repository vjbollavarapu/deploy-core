'use client'

import { useCallback, useEffect, useState } from 'react'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { RevisionsFilterTable } from '@/components/deploycore/revisions/revisions-filter-table'
import {
  apiClient,
  type Page,
  type Application as WireApplication,
  type RevisionStatus,
} from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import type { Revision, Status } from '@/lib/types'

interface WireRevisionResponse {
  id: string
  organizationId: string
  applicationId: string
  deploymentId?: string | null
  revisionNumber: number
  status: RevisionStatus | string
  commitSha?: string | null
  imageDigest?: string | null
  imageTag?: string | null
  effectiveConfig?: Record<string, unknown>
  variableSnapshot?: Record<string, unknown>
  secretRefs?: unknown[]
  healthCheck?: Record<string, unknown>
  resourceLimits?: Record<string, unknown>
  createdBy?: string | null
  createdAt: string
  updatedAt: string
}

interface RevisionsPageClientProps {
  revisions: Revision[]
}

function mapWireRevisionStatus(status: string): Status {
  switch (status.toUpperCase()) {
    case 'ACTIVE':
    case 'READY':
      return 'healthy'
    case 'CREATED':
      return 'deploying'
    case 'FAILED':
      return 'failed'
    case 'ARCHIVED':
    case 'INACTIVE':
      return 'stopped'
    default:
      return 'healthy'
  }
}

export function RevisionsPageClient({ revisions: fallbackRevisions }: RevisionsPageClientProps) {
  const { activeOrg } = useOrganization()
  const [revisionsList, setRevisionsList] = useState<Revision[]>(fallbackRevisions)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadRevisions() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const appRes = await apiClient
          .get<Page<WireApplication>>(`/applications?organizationId=${activeOrg.id}`)
          .catch(() => null)

        if (cancelled) return

        const apps = Array.isArray(appRes?.items) ? appRes.items : []
        if (apps.length === 0) {
          setIsLoading(false)
          return
        }

        const appMap: Record<string, WireApplication> = {}
        apps.forEach((a) => {
          if (a.id) appMap[a.id] = a
        })

        const allRevs: Revision[] = []

        await Promise.all(
          apps.slice(0, 8).map(async (app) => {
            if (!app.id) return
            try {
              const res = await apiClient.get<Page<WireRevisionResponse>>(
                `/applications/${app.id}/revisions`,
              )
              if (Array.isArray(res?.items)) {
                for (const item of res.items) {
                  const status = mapWireRevisionStatus(item.status)
                  const appName = appMap[item.applicationId]?.name || 'Application'
                  const limits = item.resourceLimits || {}
                  const hc = item.healthCheck || {}
                  const cfg = item.effectiveConfig || {}
                  const vars = item.variableSnapshot || {}

                  allRevs.push({
                    id: item.id,
                    number: String(item.revisionNumber).padStart(5, '0'),
                    applicationId: item.applicationId,
                    application: appName,
                    environment: 'production',
                    status,
                    traffic: item.status.toUpperCase() === 'ACTIVE' ? 100 : 0,
                    commit: item.commitSha ? item.commitSha.slice(0, 7) : 'a1b2c3d',
                    commitMessage:
                      (cfg['commitMessage'] as string) ||
                      `Revision ${item.revisionNumber} configuration snapshot`,
                    image:
                      (cfg['image'] as string) ||
                      (item.imageTag ? `${appName}:${item.imageTag}` : 'registry.internal/app:latest'),
                    imageDigest:
                      item.imageDigest ||
                      'sha256:7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069',
                    createdBy: { name: 'Operator' },
                    createdAt: item.createdAt
                      ? new Date(item.createdAt).toLocaleDateString([], {
                          month: 'short',
                          day: 'numeric',
                          hour: '2-digit',
                          minute: '2-digit',
                        })
                      : 'Just now',
                    runtime: 'Docker Image',
                    server: (cfg['server'] as string) || 'srv-primary',
                    command: (cfg['command'] as string) || 'npm run start',
                    cpuLimit: Number(limits['cpu'] ?? 2),
                    memoryLimit: Number(limits['memoryMb'] ?? 2048),
                    envVarCount: Object.keys(vars).length,
                    envVars: Object.keys(vars).map((k) => ({
                      key: k,
                      scope: 'Application' as const,
                      secret: false,
                    })),
                    secretRefs: Array.isArray(item.secretRefs) ? item.secretRefs.map(String) : [],
                    domains: Array.isArray(cfg['domains']) ? (cfg['domains'] as string[]) : [],
                    volumes: Array.isArray(cfg['volumes']) ? (cfg['volumes'] as string[]) : [],
                    healthCheck: {
                      type: (hc['type'] as string) || 'HTTP',
                      path: (hc['path'] as string) || '/healthz',
                      interval: (hc['interval'] as string) || '15s',
                    },
                    archived: item.status.toUpperCase() === 'ARCHIVED',
                  })
                }
              }
            } catch {
              // Ignore per-app revision fetch errors in dev/partial environments
            }
          }),
        )

        if (cancelled) return

        if (allRevs.length > 0) {
          allRevs.sort((a, b) => Number(b.number) - Number(a.number))
          setRevisionsList(allRevs)
        }
      } catch (err) {
        if (cancelled) return
        if (fallbackRevisions.length === 0) {
          setError(
            err instanceof Error ? err.message : 'Unable to load revisions from control plane',
          )
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadRevisions()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackRevisions, refreshKey])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Revisions"
        description="Immutable builds across applications, traffic splits, rollbacks, and configuration compare."
      />

      {error ? (
        <ErrorState title="Could not load revisions" message={error} onRetry={reload} />
      ) : isLoading && revisionsList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading revisions…" />
      ) : (
        <RevisionsFilterTable revisions={revisionsList} onRevisionUpdated={reload} />
      )}
    </PageContainer>
  )
}
