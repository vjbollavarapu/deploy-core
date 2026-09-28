import type { WireEnvironment, WireProject } from '@/lib/api'
import type {
  Application,
  DatabaseInstance,
  Deployment,
  DockerNetwork,
  DomainRecord,
  EnvVarEntry,
  Project,
  ProjectEnvironmentRef,
  SecretItem,
  Status,
  Volume,
} from '@/lib/types'
import {
  applications as rawApplications,
  databases as rawDatabases,
  deployments as rawDeployments,
  domains as rawDomains,
  envVarHierarchy as rawEnvVarHierarchy,
  networks as rawNetworks,
  projects as rawProjects,
  secrets as rawSecrets,
  volumes as rawVolumes,
} from '@/lib/mock-data'
import { getDemoFixtures, allowSyntheticFallback } from '@/lib/mock-isolation'

const applications = getDemoFixtures(rawApplications)
const databases = getDemoFixtures(rawDatabases)
const deployments = getDemoFixtures(rawDeployments)
const domains = getDemoFixtures(rawDomains)
const envVarHierarchy = getDemoFixtures(rawEnvVarHierarchy)
const networks = getDemoFixtures(rawNetworks)
const projects = getDemoFixtures(rawProjects)
const secrets = getDemoFixtures(rawSecrets)
const volumes = getDemoFixtures(rawVolumes)

export function projectDetailPath(project: Pick<Project, 'id'>): string {
  return `/projects/${project.id}`
}

export function environmentRefFor(
  project: Project,
  environmentName: string,
): ProjectEnvironmentRef | undefined {
  return project.environmentRefs?.find(
    (env) =>
      env.id === environmentName ||
      env.name === environmentName ||
      env.slug === environmentName ||
      env.slug === environmentSlug(environmentName),
  )
}

export function environmentDetailPath(project: Project, environmentName: string): string {
  const ref = environmentRefFor(project, environmentName)
  const segment = ref?.id ?? environmentSlug(environmentName)
  return `/projects/${project.id}/environments/${segment}`
}

export function wireProjectToViewModel(
  wire: WireProject,
  environments: WireEnvironment[] = [],
): Project {
  const refs: ProjectEnvironmentRef[] = []
  for (const env of environments) {
    if (!env.id) continue
    const name = env.name || env.slug || env.id
    const slug = env.slug || name.toLowerCase()
    refs.push({ id: env.id, name, slug })
  }
  return {
    id: wire.id || '',
    name: wire.name || 'Untitled',
    slug: wire.slug || '',
    description: wire.description,
    environments: refs.map((env) => env.name),
    environmentRefs: refs,
    applicationCount: 0,
    health: 'unknown',
    lastDeployment: '—',
    owner: { name: '—' },
    updatedAt: wire.updatedAt ? new Date(wire.updatedAt).toLocaleDateString() : '—',
  }
}

export function emptyProjectResources() {
  return {
    databases: [] as DatabaseInstance[],
    volumes: [] as Volume[],
    networks: [] as DockerNetwork[],
    domains: [] as DomainRecord[],
    secrets: [] as SecretItem[],
  }
}

export function findProject(projectId: string, list: Project[] = projects): Project | undefined {
  const found = list.find((p) => p.slug === projectId || p.id === projectId)
  if (found) return found
  if (allowSyntheticFallback() && projectId && projectId !== 'undefined') {
    return {
      id: projectId,
      name: projectId.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
      slug: projectId.toLowerCase(),
      environments: ['production', 'staging'],
      applicationCount: 0,
      health: 'healthy',
      lastDeployment: 'Never',
      owner: { name: 'DeployCore Admin' },
      updatedAt: 'Just now',
    }
  }
  return undefined
}

export function resolveEnvironment(project: Project, environmentId: string): string | undefined {
  const found = project.environments.find(
    (env) => env.toLowerCase() === environmentId.toLowerCase() || env === environmentId,
  )
  if (found) return found
  if (environmentId && environmentId !== 'undefined') {
    return environmentId.charAt(0).toUpperCase() + environmentId.slice(1)
  }
  return undefined
}

export function environmentSlug(environment: string): string {
  return environment.toLowerCase()
}

export function getProjectApplications(project: Project): Application[] {
  return applications.filter((app) => app.projectId === project.id)
}

export function getProjectDeployments(project: Project): Deployment[] {
  return deployments.filter((d) => d.project === project.name)
}

export function getEnvironmentApplications(project: Project, environment: string): Application[] {
  return getProjectApplications(project).filter((app) => app.environment === environment)
}

export function getEnvironmentDatabases(project: Project, environment: string): DatabaseInstance[] {
  return databases.filter((db) => db.project === project.name && db.environment === environment)
}

export function getEnvironmentDomains(project: Project, environment: string): DomainRecord[] {
  const appIds = new Set(getEnvironmentApplications(project, environment).map((app) => app.id))
  return domains.filter((domain) => domain.environment === environment && appIds.has(domain.applicationId))
}

export function getEnvironmentVariables(project: Project, environment: string): EnvVarEntry[] {
  return envVarHierarchy.filter((entry) => {
    if (entry.scope === 'Organization') return true
    if (entry.scope === 'Project') return entry.source === project.name
    if (entry.scope === 'Environment') return entry.source === environment
    return false
  })
}

export function getEnvironmentSecrets(project: Project, environment: string): SecretItem[] {
  const appNames = new Set(getEnvironmentApplications(project, environment).map((app) => app.name))
  return secrets.filter(
    (secret) =>
      secret.scope === 'Project' ||
      secret.applications.some((app) => appNames.has(app)),
  )
}

export function getEnvironmentHealth(project: Project, environment: string): Status {
  const apps = getEnvironmentApplications(project, environment)
  const dbs = getEnvironmentDatabases(project, environment)
  const statuses = [...apps.map((a) => a.status), ...dbs.map((d) => d.status)]
  if (statuses.some((s) => s === 'failed' || s === 'offline')) return 'failed'
  if (statuses.some((s) => s === 'degraded')) return 'degraded'
  if (statuses.some((s) => s === 'deploying' || s === 'pending' || s === 'queued')) return 'deploying'
  if (statuses.length === 0) return 'unknown'
  return 'healthy'
}

export function getProjectResources(project: Project) {
  const apps = getProjectApplications(project)
  const appNames = new Set(apps.map((a) => a.name))
  return {
    databases: databases.filter((db) => db.project === project.name),
    volumes: volumes.filter((volume) =>
      apps.some((app) => volume.attachedResource.includes(app.name) || volume.name.includes(project.slug)),
    ),
    networks: networks.filter((network) => network.project === project.name),
    domains: domains.filter((domain) => appNames.has(domain.application) || apps.some((a) => a.id === domain.applicationId)),
    secrets: secrets.filter(
      (secret) => secret.scope === 'Project' || secret.applications.some((name) => appNames.has(name)),
    ),
  }
}
