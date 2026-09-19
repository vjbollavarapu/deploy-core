import { notFound } from 'next/navigation'
import { EnvironmentDetailClient } from '@/components/deploycore/projects/environment-detail-client'
import {
  findProject,
  getEnvironmentApplications,
  getEnvironmentDatabases,
  getEnvironmentDomains,
  getEnvironmentHealth,
  getEnvironmentSecrets,
  getEnvironmentVariables,
  resolveEnvironment,
} from '@/lib/projects'

export default async function ProjectEnvironmentPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>
}) {
  const { projectId, environmentId } = await params
  const project = findProject(projectId)
  if (!project) notFound()

  const environment = resolveEnvironment(project, environmentId)
  if (!environment) notFound()

  const applications = getEnvironmentApplications(project, environment)
  const databases = getEnvironmentDatabases(project, environment)
  const variables = getEnvironmentVariables(project, environment)
  const secrets = getEnvironmentSecrets(project, environment)
  const domains = getEnvironmentDomains(project, environment)
  const health = getEnvironmentHealth(project, environment)

  return (
    <EnvironmentDetailClient
      project={project}
      environment={environment}
      health={health}
      applications={applications}
      databases={databases}
      variables={variables}
      secrets={secrets}
      domains={domains}
    />
  )
}
