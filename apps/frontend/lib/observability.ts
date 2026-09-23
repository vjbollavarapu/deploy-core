import {
  activityFeed as rawActivityFeed,
  applications as rawApplications,
  certificateWarnings as rawCertificateWarnings,
  containers as rawContainers,
  databases as rawDatabases,
  deployments as rawDeployments,
  generateLogLines,
  incidents as rawIncidents,
  servers as rawServers,
} from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const applications = getDemoFixtures(rawApplications)
const containers = getDemoFixtures(rawContainers)
const databases = getDemoFixtures(rawDatabases)
const servers = getDemoFixtures(rawServers)
const activityFeed = getDemoFixtures(rawActivityFeed)
const incidents = getDemoFixtures(rawIncidents)
const certificateWarnings = getDemoFixtures(rawCertificateWarnings)
const deployments = getDemoFixtures(rawDeployments)
import type {
  Application,
  Container,
  DatabaseInstance,
  LogLine,
  PlatformEvent,
  Server,
} from '@/lib/types'

export const LOG_LEVELS = ['info', 'debug', 'warn', 'error'] as const

export type LogLevel = (typeof LOG_LEVELS)[number]

export interface LogExplorerFilters {
  application: string
  environment: string
  revision: string
  container: string
  source: 'all' | 'application' | 'server' | 'database'
}

export function listLogEnvironments(): string[] {
  return [...new Set(applications.map((app) => app.environment))].sort()
}

export function listLogRevisions(applicationName?: string): string[] {
  const rows = containers.filter(
    (c) => !applicationName || applicationName === 'all' || c.application === applicationName,
  )
  return [...new Set(rows.map((c) => c.revision))].sort()
}

export function listLogContainers(opts?: {
  application?: string
  environment?: string
  revision?: string
}): string[] {
  const envApps = new Set(
    applications
      .filter((app) => !opts?.environment || opts.environment === 'all' || app.environment === opts.environment)
      .map((app) => app.name),
  )
  return containers
    .filter((c) => {
      if (opts?.application && opts.application !== 'all' && c.application !== opts.application) return false
      if (opts?.revision && opts.revision !== 'all' && c.revision !== opts.revision) return false
      if (opts?.environment && opts.environment !== 'all' && !envApps.has(c.application)) return false
      return true
    })
    .map((c) => c.name)
    .sort()
}

/** Fleet log corpus built from applications, servers, and databases already in mock API data. */
export function getFleetLogLines(countPerSource = 40): LogLine[] {
  const appLines = applications.flatMap((app) => {
    const appContainers = containers.filter((c) => c.applicationId === app.id)
    const sources = appContainers.length > 0 ? appContainers : [null]
    return sources.flatMap((container, index) =>
      enrichLogLines(
        generateLogLines(container?.name ?? app.name, Math.max(8, Math.floor(countPerSource / 2))),
        {
          application: app.name,
          environment: app.environment,
          revision: container?.revision ?? app.revision,
          container: container?.name ?? app.name,
          idPrefix: `app-${app.id}-${index}`,
        },
      ),
    )
  })

  const serverLines = servers.flatMap((server) =>
    enrichLogLines(generateLogLines(`agent@${server.name}`, 20), {
      application: undefined,
      environment: undefined,
      revision: undefined,
      container: `agent@${server.name}`,
      idPrefix: `srv-${server.id}`,
    }),
  )

  const databaseLines = databases.flatMap((database) =>
    enrichLogLines(generateLogLines(`postgres@${database.name}`, 20), {
      application: undefined,
      environment: database.environment,
      revision: undefined,
      container: `postgres@${database.name}`,
      idPrefix: `db-${database.id}`,
    }),
  )

  return [...appLines, ...serverLines, ...databaseLines].sort((a, b) =>
    a.timestamp < b.timestamp ? -1 : 1,
  )
}

export function filterFleetLogs(lines: LogLine[], filters: LogExplorerFilters): LogLine[] {
  return lines.filter((line) => {
    if (filters.source === 'application' && !line.application) return false
    if (filters.source === 'server' && !line.container.startsWith('agent@')) return false
    if (filters.source === 'database' && !line.container.startsWith('postgres@')) return false

    if (filters.application !== 'all' && line.application !== filters.application) return false
    if (filters.environment !== 'all' && line.environment !== filters.environment) return false
    if (filters.revision !== 'all' && line.revision !== filters.revision) return false
    if (filters.container !== 'all' && line.container !== filters.container) return false
    return true
  })
}

export function getApplicationLogLines(application: Application, count = 100): LogLine[] {
  const appContainers = containers.filter((c) => c.applicationId === application.id)
  if (appContainers.length === 0) {
    return enrichLogLines(generateLogLines(application.name, count), {
      application: application.name,
      environment: application.environment,
      revision: application.revision,
      container: application.name,
      idPrefix: `app-${application.id}`,
    })
  }
  const per = Math.max(20, Math.floor(count / appContainers.length))
  return appContainers.flatMap((container, index) =>
    enrichLogLines(generateLogLines(container.name, per), {
      application: application.name,
      environment: application.environment,
      revision: container.revision,
      container: container.name,
      idPrefix: `app-${application.id}-${index}`,
    }),
  )
}

export function getServerLogLines(server: Server, count = 120): LogLine[] {
  return enrichLogLines(generateLogLines(`agent@${server.name}`, count), {
    container: `agent@${server.name}`,
    idPrefix: `srv-${server.id}`,
  })
}

export function getDatabaseLogLines(database: DatabaseInstance, count = 120): LogLine[] {
  return enrichLogLines(generateLogLines(`postgres@${database.name}`, count), {
    environment: database.environment,
    container: `postgres@${database.name}`,
    idPrefix: `db-${database.id}`,
  })
}

function enrichLogLines(
  lines: LogLine[],
  meta: {
    application?: string
    environment?: string
    revision?: string
    container: string
    idPrefix: string
  },
): LogLine[] {
  return lines.map((line, index) => ({
    ...line,
    id: `${meta.idPrefix}-${index}`,
    container: meta.container,
    application: meta.application,
    environment: meta.environment,
    revision: meta.revision,
  }))
}

/** Metrics derived only from fields already present on mock API entities. */
export function getFleetMetricSummaries() {
  const appCpu =
    applications.length === 0
      ? 0
      : Math.round(applications.reduce((sum, app) => sum + app.cpu, 0) / applications.length)
  const appRam =
    applications.length === 0
      ? 0
      : Math.round(applications.reduce((sum, app) => sum + app.memory, 0) / applications.length)
  const serverCpu =
    servers.length === 0
      ? 0
      : Math.round(servers.reduce((sum, server) => sum + server.cpu, 0) / servers.length)
  const serverRam =
    servers.length === 0
      ? 0
      : Math.round(servers.reduce((sum, server) => sum + server.memory, 0) / servers.length)
  const storage =
    servers.length === 0
      ? 0
      : Math.round(servers.reduce((sum, server) => sum + server.disk, 0) / servers.length)
  const restarts = containers.reduce((sum, c) => sum + c.restarts, 0)

  return {
    cpu: appCpu,
    ram: appRam,
    storage,
    serverCpu,
    serverRam,
    restarts,
    applicationCount: applications.length,
    serverCount: servers.length,
    containerCount: containers.length,
    /** Network RX/TX is not present on current API mock entities. */
    networkAvailable: false,
  }
}

export function getApplicationMetricRows(application: Application) {
  const rows = containers.filter((c) => c.applicationId === application.id)
  return {
    application,
    containers: rows,
    cpu: application.cpu,
    ram: application.memory,
    cpuLimit: application.cpuLimit,
    memoryLimit: application.memoryLimit,
    uptime: application.uptime,
    restarts: rows.reduce((sum, c) => sum + c.restarts, 0),
    networkAvailable: false,
    storageAvailable: false,
  }
}

export function getServerMetricRows() {
  return servers.map((server) => ({
    id: server.id,
    name: server.name,
    cpu: server.cpu,
    ram: server.memory,
    storage: server.disk,
    uptime: server.uptime,
    containers: server.containers,
    status: server.status,
    networkAvailable: false,
  }))
}

export function getContainerMetricRows(): Array<
  Container & { uptime?: string; networkAvailable: false; storageAvailable: false }
> {
  return containers.map((container) => ({
    ...container,
    networkAvailable: false as const,
    storageAvailable: false as const,
  }))
}

/** Platform events assembled from existing activity, incidents, certificates, and deployments. */
export function getPlatformEvents(): PlatformEvent[] {
  const fromActivity: PlatformEvent[] = activityFeed.map((item) => ({
    id: item.id,
    timestamp: item.timestamp,
    category: categorizeActivity(item.action),
    actor: item.actor,
    action: item.action,
    target: item.target,
    tone: item.tone,
  }))

  const fromIncidents: PlatformEvent[] = incidents.map((incident) => ({
    id: incident.id,
    timestamp: incident.startedAt,
    category: 'incident',
    actor: 'System',
    action: `${incident.status.toLowerCase()} incident`,
    target: incident.title,
    tone: incident.severity === 'critical' ? 'critical' : 'warning',
  }))

  const fromCerts: PlatformEvent[] = certificateWarnings.map((cert, index) => ({
    id: `cert-evt-${index}`,
    timestamp: `expires in ${cert.expiresIn}`,
    category: 'certificate',
    actor: 'System',
    action: 'flagged certificate',
    target: cert.domain,
    tone: cert.status === 'critical' ? 'critical' : 'warning',
  }))

  const fromDeployments: PlatformEvent[] = deployments.slice(0, 8).map((deployment) => ({
    id: `dep-evt-${deployment.id}`,
    timestamp: deployment.startedAt,
    category: 'deployment',
    actor: deployment.author.name,
    action:
      deployment.status === 'failed'
        ? 'deployment failed for'
        : deployment.status === 'running' || deployment.status === 'deploying'
          ? 'started deployment for'
          : 'deployed',
    target: `${deployment.application} #${deployment.number}`,
    tone:
      deployment.status === 'failed'
        ? 'critical'
        : deployment.status === 'running' || deployment.status === 'deploying'
          ? 'info'
          : 'success',
    project: deployment.project,
    environment: deployment.environment,
  }))

  return [...fromActivity, ...fromIncidents, ...fromCerts, ...fromDeployments]
}

function categorizeActivity(action: string): PlatformEvent['category'] {
  if (action.includes('backup')) return 'backup'
  if (action.includes('secret')) return 'security'
  if (action.includes('certificate')) return 'certificate'
  if (action.includes('server')) return 'infrastructure'
  if (action.includes('deploy')) return 'deployment'
  return 'infrastructure'
}

/** Lightweight series from current entity values — not invented backend counters. */
export function applicationMetricSeries(application: Application, points = 24) {
  return Array.from({ length: points }).map((_, index) => {
    const wave = Math.sin(index / 3) * 5
    return {
      t: `${points - index}m`,
      cpu: Math.min(100, Math.max(0, Math.round(application.cpu + wave))),
      memory: Math.min(100, Math.max(0, Math.round(application.memory + wave * 0.7))),
    }
  })
}
