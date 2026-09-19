import {
  activityFeed,
  applications,
  deployments,
  domains,
  envVarHierarchy,
  networks,
  revisions,
  secrets,
  volumes,
} from '@/lib/mock-data'
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

export function findApplication(applicationId: string): Application | undefined {
  return applications.find((app) => app.id === applicationId)
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
