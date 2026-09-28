'use client'

import { useCallback } from 'react'
import { useParams } from 'next/navigation'
import { Boxes } from 'lucide-react'
import { EnvironmentDetailClient } from '@/components/deploycore/projects/environment-detail-client'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { apiClient, ApiError, type WireEnvironment, type WireProject } from '@/lib/api'
import { useApiQuery } from '@/hooks/use-api-query'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import {
  findProject,
  getEnvironmentApplications,
  getEnvironmentDatabases,
  getEnvironmentDomains,
  getEnvironmentHealth,
  getEnvironmentSecrets,
  getEnvironmentVariables,
  resolveEnvironment,
  wireProjectToViewModel,
} from '@/lib/projects'

type ProjectResponse = { project?: WireProject }
type EnvironmentResponse = { environment?: WireEnvironment }

interface LoadedEnvironment {
  project: ReturnType<typeof wireProjectToViewModel>
  environmentName: string
  environmentId: string
}

export default function ProjectEnvironmentPage() {
  const params = useParams<{ projectId: string; environmentId: string }>()
  const projectId = params.projectId
  const environmentId = params.environmentId
  if (!projectId || !environmentId) return null
  return (
    <EnvironmentDetailBody
      key={`${projectId}:${environmentId}`}
      projectId={projectId}
      environmentId={environmentId}
    />
  )
}

function EnvironmentDetailBody({
  projectId,
  environmentId,
}: {
  projectId: string
  environmentId: string
}) {
  const demo = isDemoModeEnabled()

  const fetcher = useCallback(async (): Promise<LoadedEnvironment> => {
    if (demo) {
      const local = findProject(projectId)
      if (!local) {
        throw new ApiError(404, 'Project not found', 'RESOURCE_NOT_FOUND')
      }
      const environment = resolveEnvironment(local, environmentId)
      if (!environment) {
        throw new ApiError(404, 'Environment not found', 'RESOURCE_NOT_FOUND')
      }
      return {
        project: local,
        environmentName: environment,
        environmentId,
      }
    }

    const [projectRes, envRes] = await Promise.all([
      apiClient.get<ProjectResponse>(`/projects/${projectId}`),
      apiClient.get<EnvironmentResponse>(`/environments/${environmentId}`),
    ])
    const wireProject = projectRes.project
    const wireEnv = envRes.environment
    if (!wireProject?.id || !wireEnv?.id) {
      throw new ApiError(404, 'Environment not found', 'RESOURCE_NOT_FOUND')
    }
    if (wireEnv.projectId && wireEnv.projectId !== wireProject.id) {
      throw new ApiError(404, 'Environment not found', 'RESOURCE_NOT_FOUND')
    }
    const project = wireProjectToViewModel(wireProject, [wireEnv])
    return {
      project,
      environmentName: wireEnv.name || wireEnv.slug || wireEnv.id,
      environmentId: wireEnv.id,
    }
  }, [demo, environmentId, projectId])

  const { data, isLoading, error, errorCode, reload } = useApiQuery(fetcher)

  if (isLoading && !data) {
    return (
      <PageContainer density="wide">
        <LoadingState variant="page" label="Loading environment…" />
      </PageContainer>
    )
  }

  if (errorCode === 'RESOURCE_NOT_FOUND') {
    return (
      <PageContainer density="wide">
        <EmptyState
          icon={Boxes}
          title="Environment not found"
          description="This environment does not exist or you do not have access to it."
        />
      </PageContainer>
    )
  }

  if (error || !data) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load environment"
          message={error || 'Unable to load this environment from the Control Plane'}
          onRetry={reload}
        />
      </PageContainer>
    )
  }

  if (demo) {
    return (
      <EnvironmentDetailClient
        project={data.project}
        environment={data.environmentName}
        environmentId={data.environmentId}
        health={getEnvironmentHealth(data.project, data.environmentName)}
        applications={getEnvironmentApplications(data.project, data.environmentName)}
        databases={getEnvironmentDatabases(data.project, data.environmentName)}
        variables={getEnvironmentVariables(data.project, data.environmentName)}
        secrets={getEnvironmentSecrets(data.project, data.environmentName)}
        domains={getEnvironmentDomains(data.project, data.environmentName)}
      />
    )
  }

  return (
    <EnvironmentDetailClient
      project={data.project}
      environment={data.environmentName}
      environmentId={data.environmentId}
      health="unknown"
      applications={[]}
      databases={[]}
      variables={[]}
      secrets={[]}
      domains={[]}
    />
  )
}
