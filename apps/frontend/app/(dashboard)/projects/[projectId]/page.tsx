import { notFound } from 'next/navigation'
import { ProjectDetailClient } from '@/components/deploycore/projects/project-detail-client'
import {
  findProject,
  getEnvironmentHealth,
  getProjectApplications,
  getProjectDeployments,
  getProjectResources,
} from '@/lib/projects'

export default async function ProjectDetailPage({
  params,
}: {
  params: Promise<{ projectId: string }>
}) {
  const { projectId } = await params
  const project = findProject(projectId)
  if (!project) notFound()

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
    />
  )
}
