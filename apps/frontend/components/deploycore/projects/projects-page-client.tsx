'use client'

import { useCallback, useEffect, useState } from 'react'
import { NewProjectButton } from '@/components/deploycore/projects/project-settings-panel'
import { ProjectsTable } from '@/components/deploycore/projects/projects-table'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { apiClient, type Page, type WireProject, type WireEnvironment } from '@/lib/api'
import { useAuth, useOrganization } from '@/lib/auth-context'
import type { Project } from '@/lib/types'

interface ProjectsPageClientProps {
  projects: Project[]
}

export function ProjectsPageClient({ projects: fallbackProjects }: ProjectsPageClientProps) {
  const { activeOrg } = useOrganization()
  const { user } = useAuth()
  const [projectList, setProjectList] = useState<Project[]>(fallbackProjects)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadProjects() {
      if (!activeOrg?.id) return
      setIsLoading(true)
      setError(null)

      try {
        const res = await apiClient.get<Page<WireProject>>(
          `/projects?organizationId=${activeOrg.id}`,
        )

        if (cancelled) return

        if (Array.isArray(res?.items)) {
          // If items returned from API, map them to UI view model
          const mapped: Project[] = await Promise.all(
            res.items.map(async (p): Promise<Project> => {
              let envs = ['production', 'staging']
              if (p.id) {
                try {
                  const envRes = await apiClient.get<Page<WireEnvironment>>(
                    `/projects/${p.id}/environments`,
                  )
                  if (Array.isArray(envRes?.items) && envRes.items.length > 0) {
                    envs = envRes.items.map((e) => e.name || e.slug || 'production')
                  }
                } catch {
                  // Fall back to default envs if unconfigured
                }
              }
              return {
                id: p.id || p.slug || '',
                name: p.name || 'Untitled',
                slug: p.slug || '',
                description: p.description,
                environments: envs,
                applicationCount: 0,
                health: 'healthy',
                lastDeployment: 'Just now',
                owner: {
                  name: user?.displayName || user?.email || 'Unknown',
                },
                updatedAt: p.updatedAt
                  ? new Date(p.updatedAt).toLocaleDateString()
                  : 'Today',
              }
            }),
          )

          if (!cancelled) {
            setProjectList(mapped)
          }
        } else {
          if (!cancelled) {
            setProjectList([])
          }
        }
      } catch (err) {
        if (cancelled) return
        // Prefer ApiError.message (already operator-safe); never dump raw HTML bodies.
        const message =
          err instanceof Error && err.message && !err.message.includes('<html')
            ? err.message
            : 'Could not load projects. The API request failed. Please retry.'
        setError(message)
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadProjects()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, fallbackProjects, refreshKey, user?.displayName, user?.email])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Projects"
        description="Group related applications, environments, and shared infrastructure."
        actions={<NewProjectButton onCreated={reload} />}
      />

      {error ? (
        <ErrorState
          title="Could not load projects"
          message={error}
          onRetry={reload}
        />
      ) : isLoading && projectList.length === 0 ? (
        <LoadingState variant="table" rows={6} label="Loading projects…" />
      ) : (
        <ProjectsTable
          projects={projectList}
          onCreated={reload}
        />
      )}
    </PageContainer>
  )
}
