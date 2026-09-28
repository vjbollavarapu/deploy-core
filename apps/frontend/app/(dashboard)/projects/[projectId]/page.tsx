'use client'

import { useCallback } from 'react'
import { useParams } from 'next/navigation'
import { Boxes } from 'lucide-react'
import { ProjectDetailClient } from '@/components/deploycore/projects/project-detail-client'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { apiClient, ApiError, type Page, type WireEnvironment, type WireProject } from '@/lib/api'
import { useApiQuery } from '@/hooks/use-api-query'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import {
  emptyProjectResources,
  findProject,
  getEnvironmentHealth,
  getProjectApplications,
  getProjectDeployments,
  getProjectResources,
  wireProjectToViewModel,
} from '@/lib/projects'
import type { Project } from '@/lib/types'

type ProjectResponse = { project?: WireProject }

export default function ProjectDetailPage() {
  const params = useParams<{ projectId: string }>()
  const projectId = params.projectId
  if (!projectId) return null
  return <ProjectDetailBody key={projectId} projectId={projectId} />
}

function ProjectDetailBody({ projectId }: { projectId: string }) {
  const demo = isDemoModeEnabled()

  const fetcher = useCallback(async (): Promise<Project> => {
    if (demo) {
      const local = findProject(projectId)
      if (!local) {
        throw new ApiError(404, 'Project not found', 'RESOURCE_NOT_FOUND')
      }
      return local
    }

    const [projectRes, envRes] = await Promise.all([
      apiClient.get<ProjectResponse>(`/projects/${projectId}`),
      apiClient.get<Page<WireEnvironment>>(`/projects/${projectId}/environments`),
    ])
    if (!projectRes.project?.id) {
      throw new ApiError(404, 'Project not found', 'RESOURCE_NOT_FOUND')
    }
    return wireProjectToViewModel(projectRes.project, envRes.items ?? [])
  }, [demo, projectId])

  const { data: project, isLoading, error, errorCode, reload } = useApiQuery(fetcher)

  if (isLoading && !project) {
    return (
      <PageContainer density="wide">
        <LoadingState variant="page" label="Loading project…" />
      </PageContainer>
    )
  }

  if (errorCode === 'RESOURCE_NOT_FOUND') {
    return (
      <PageContainer density="wide">
        <EmptyState
          icon={Boxes}
          title="Project not found"
          description="This project does not exist or you do not have access to it."
        />
      </PageContainer>
    )
  }

  if (error || !project) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load project"
          message={error || 'Unable to load this project from the Control Plane'}
          onRetry={reload}
        />
      </PageContainer>
    )
  }

  if (demo) {
    const applications = getProjectApplications(project)
    const deployments = getProjectDeployments(project)
    const resources = getProjectResources(project)
    const environmentHealth = Object.fromEntries(
      project.environments.map((env) => [env, getEnvironmentHealth(project, env)]),
    )
    return (
      <ProjectDetailClient
        project={project}
        applications={applications}
        deployments={deployments}
        resources={resources}
        environmentHealth={environmentHealth}
        onRefresh={reload}
      />
    )
  }

  const environmentHealth = Object.fromEntries(
    project.environments.map((env) => [env, 'unknown' as const]),
  )

  return (
    <ProjectDetailClient
      project={project}
      applications={[]}
      deployments={[]}
      resources={emptyProjectResources()}
      environmentHealth={environmentHealth}
      onRefresh={reload}
    />
  )
}
