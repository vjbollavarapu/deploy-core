'use client'

import { useCallback, useEffect, useState } from 'react'
import { ApplicationsFilterTable } from '@/components/deploycore/applications/applications-filter-table'
import { CreateApplicationWizard } from '@/components/deploycore/applications/create-application-wizard'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { apiClient, type Page, type Application as WireApplication, type WireProject } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import type { Application, RuntimeType, Status } from '@/lib/types'

interface ApplicationsPageClientProps {
  applications: Application[]
}

function mapRuntimeType(rawType?: string): RuntimeType {
  switch (rawType) {
    case 'API':
      return 'API'
    case 'WORKER':
      return 'Worker'
    case 'SCHEDULED_JOB':
      return 'Scheduled Job'
    case 'STATIC_SITE':
      return 'Static Site'
    case 'DOCKER_COMPOSE':
      return 'Docker Compose'
    case 'DOCKER_IMAGE':
      return 'Docker Image'
    default:
      return 'Web Service'
  }
}

export function ApplicationsPageClient({ applications: fallbackApplications }: ApplicationsPageClientProps) {
  const { activeOrg } = useOrganization()
  const [applicationList, setApplicationList] = useState<Application[]>(fallbackApplications)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadApplications() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const [appRes, projRes] = await Promise.all([
          apiClient.get<Page<WireApplication>>(`/applications?organizationId=${activeOrg.id}`),
          apiClient.get<Page<WireProject>>(`/projects?organizationId=${activeOrg.id}`).catch(() => null),
        ])

        if (cancelled) return

        const projectNamesById: Record<string, string> = {}
        if (Array.isArray(projRes?.items)) {
          for (const proj of projRes.items) {
            if (proj.id && proj.name) {
              projectNamesById[proj.id] = proj.name
            }
          }
        }

        if (Array.isArray(appRes?.items)) {
          const mapped: Application[] = appRes.items.map((app) => {
            const projectName = (app.projectId && projectNamesById[app.projectId]) || 'Core Platform'
            const statusStr = (app.status?.toLowerCase() as Status) || 'healthy'

            return {
              id: app.id || app.slug || '',
              name: app.name || 'Untitled',
              projectId: app.projectId || '',
              project: projectName,
              environment: 'production',
              runtime: mapRuntimeType(app.type),
              server: app.targetServerId || 'srv-primary',
              revision: 'rev-init',
              status: statusStr,
              domain: app.slug ? `${app.slug}.deploycore.app` : '',
              lastDeployment: 'Just now',
              repo: app.config?.repositoryUrl || '—',
              branch: app.config?.gitBranch || 'main',
              commit: '—',
              commitMessage: '—',
              cpu: 10,
              cpuLimit: (app.config?.cpuLimitMillis || 1000) / 1000,
              memory: 20,
              memoryLimit: Math.round((app.config?.memoryLimitBytes || 536870912) / (1024 * 1024)),
              instances: app.config?.desiredReplicas || 1,
              uptime: '1d',
            }
          })

          if (!cancelled) {
            setApplicationList(mapped)
          }
        } else {
          if (!cancelled) {
            setApplicationList([])
          }
        }
      } catch (err) {
        if (cancelled) return
        setError(
          err instanceof Error ? err.message : 'Unable to load applications from control plane',
        )
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadApplications()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackApplications, refreshKey])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Applications"
        description="Workloads running across projects, environments, and servers."
        actions={<CreateApplicationWizard onSuccess={reload} />}
      />

      {error ? (
        <ErrorState
          title="Could not load applications"
          message={error}
          onRetry={reload}
        />
      ) : isLoading && applicationList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading applications…" />
      ) : (
        <ApplicationsFilterTable applications={applicationList} />
      )}
    </PageContainer>
  )
}
