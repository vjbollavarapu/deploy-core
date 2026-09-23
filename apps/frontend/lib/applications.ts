import {
  activityFeed as rawActivityFeed,
  applications as rawApplications,
  deployments as rawDeployments,
  domains as rawDomains,
  envVarHierarchy as rawEnvVarHierarchy,
  networks as rawNetworks,
  revisions as rawRevisions,
  secrets as rawSecrets,
  volumes as rawVolumes,
} from '@/lib/mock-data'
import { getDemoFixtures, allowSyntheticFallback } from '@/lib/mock-isolation'

const activityFeed = getDemoFixtures(rawActivityFeed)
const applications = getDemoFixtures(rawApplications)
const deployments = getDemoFixtures(rawDeployments)
const domains = getDemoFixtures(rawDomains)
const envVarHierarchy = getDemoFixtures(rawEnvVarHierarchy)
const networks = getDemoFixtures(rawNetworks)
const revisions = getDemoFixtures(rawRevisions)
const secrets = getDemoFixtures(rawSecrets)
const volumes = getDemoFixtures(rawVolumes)
import { getApplicationLogLines } from '@/lib/observability'
import { getRevisionsForApplication } from '@/lib/revisions'
import type {
  ActivityItem,
  Application,
  Deployment,
  DomainRecord,
  EnvVarEntry,
  LogLine,
  Revision,
  SecretItem,
  Volume,
} from '@/lib/types'
import type { DockerNetwork } from '@/lib/types'

export function findApplication(applicationId: string, list: Application[] = applications): Application | undefined {
  const found = list.find(
    (app) => app.id === applicationId || app.name.toLowerCase() === applicationId.toLowerCase(),
  )
  if (found) return found
  if (allowSyntheticFallback() && applicationId && applicationId !== 'undefined') {
    return {
      id: applicationId,
      name: applicationId.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
      projectId: 'proj-ecommerce-core',
      project: 'Core Platform',
      environment: 'production',
      runtime: 'Web Service',
      server: 'srv-hetzner-fsn1-01',
      revision: 'rev-init',
      status: 'running',
      domain: `${applicationId}.deploycore.app`,
      lastDeployment: 'Just now',
      repo: 'github.com/deploycore/service',
      branch: 'main',
      commit: 'a1b2c3d',
      commitMessage: 'Initial release',
      cpu: 15,
      cpuLimit: 1,
      memory: 30,
      memoryLimit: 512,
      instances: 1,
      uptime: '1d',
    }
  }
  return undefined
}

export function getApplicationDeployments(application: Application): Deployment[] {
  return deployments.filter((d) => d.applicationId === application.id)
}

export function getApplicationRevisions(application: Application): Revision[] {
  return getRevisionsForApplication(application.id, revisions)
}

export function getApplicationDomains(application: Application): DomainRecord[] {
  return domains.filter(
    (d) => d.applicationId === application.id || d.application === application.name,
  )
}

export function getApplicationSecrets(application: Application): SecretItem[] {
  return secrets.filter((secret) => secret.applications.includes(application.name))
}

export function getApplicationVariables(application: Application): EnvVarEntry[] {
  return envVarHierarchy.filter(
    (entry) =>
      entry.scope === 'Application' && entry.source === application.name
      || entry.scope === 'Environment' && entry.source === application.environment
      || entry.scope === 'Project' && entry.source === application.project
      || entry.scope === 'Organization',
  )
}

export function getApplicationVolumes(application: Application): Volume[] {
  return volumes.filter(
    (volume) =>
      volume.attachedResource === application.name ||
      volume.attachedResource.includes(application.name),
  )
}

export function getApplicationNetworks(application: Application): DockerNetwork[] {
  return networks.filter(
    (network) =>
      network.project === application.project &&
      network.environment === application.environment &&
      (network.connectedServices.length === 0 ||
        network.connectedServices.includes(application.name)),
  )
}

export function getApplicationActivity(application: Application): ActivityItem[] {
  const needle = application.name.toLowerCase()
  return activityFeed.filter(
    (item) =>
      item.target.toLowerCase().includes(needle) ||
      item.action.toLowerCase().includes(needle),
  )
}

export function getApplicationLogs(application: Application, count = 40): LogLine[] {
  return getApplicationLogLines(application, count)
}

export function getLatestDeployment(application: Application): Deployment | undefined {
  return getApplicationDeployments(application)[0]
}

export function getPrimaryDomain(application: Application): DomainRecord | undefined {
  const appDomains = getApplicationDomains(application)
  return appDomains.find((d) => d.primary) ?? appDomains[0]
}
