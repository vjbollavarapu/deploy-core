import {
  activityFeed as mockActivityFeed,
  applications as mockApplications,
  backupStatus as mockBackupStatus,
  certificateWarnings as mockCertificateWarnings,
  databases as mockDatabases,
  deployments as mockDeployments,
  incidents as mockIncidents,
  servers as mockServers,
} from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'
import type { Application, Deployment, Server, Status } from '@/lib/types'

const ATTENTION_STATUSES: Status[] = ['failed', 'degraded', 'offline', 'stopped', 'pending']

function average(values: number[]) {
  if (values.length === 0) return 0
  return Math.round(values.reduce((sum, value) => sum + value, 0) / values.length)
}

/** Weighted fleet utilisation across online / degraded / maintenance hosts. */
function fleetAverage(selector: (server: Server) => number | null) {
  const servers = getDemoFixtures(mockServers)
  const active = servers.filter((server) => server.status !== 'offline')
  if (active.length === 0) return 0
  return average(active.map((s) => selector(s) ?? 0))
}

/**
 * Compact sparkline points derived from current fleet load.
 * Deterministic so SSR and client match; not decorative random noise.
 */
function sparklineFromBaseline(baseline: number, points = 12): { t: string; value: number }[] {
  const pattern = [0.86, 0.9, 0.88, 0.94, 0.97, 1, 0.95, 0.92, 0.98, 1.03, 0.99, 1]
  return pattern.slice(0, points).map((factor, index) => ({
    t: `${points - index}h`,
    value: Math.min(100, Math.max(0, Math.round(baseline * factor))),
  }))
}

export function getInfrastructureStatus() {
  const servers = getDemoFixtures(mockServers)
  const applications = getDemoFixtures(mockApplications)
  const deployments = getDemoFixtures(mockDeployments)
  const databases = getDemoFixtures(mockDatabases)

  const serversOnline = servers.filter((s) => s.status === 'running' || s.status === 'degraded').length
  const applicationsRunning = applications.filter(
    (a) => a.status === 'healthy' || a.status === 'running' || a.status === 'deploying',
  ).length
  const activeDeployments = deployments.filter(
    (d) => d.status === 'deploying' || d.status === 'queued' || d.status === 'pending',
  ).length
  const databasesHealthy = databases.filter((d) => d.status === 'healthy' || d.status === 'running').length
  const failures =
    applications.filter((a) => a.status === 'failed').length +
    deployments.filter((d) => d.status === 'failed').length +
    servers.filter((s) => s.status === 'offline' || s.status === 'failed').length
  const agentConnections = servers.filter((s) => s.status !== 'offline').length

  return {
    serversOnline: { value: serversOnline, total: servers.length },
    applicationsRunning: { value: applicationsRunning, total: applications.length },
    activeDeployments: { value: activeDeployments },
    databasesHealthy: { value: databasesHealthy, total: databases.length },
    failures: { value: failures },
    agentConnections: { value: agentConnections, total: servers.length },
  }
}

export function getResourceUtilisation() {
  const cpu = fleetAverage((s) => s.cpu)
  const ram = fleetAverage((s) => s.memory)
  const storage = fleetAverage((s) => s.disk)

  return {
    cpu: { current: cpu, series: sparklineFromBaseline(cpu) },
    ram: { current: ram, series: sparklineFromBaseline(ram) },
    storage: { current: storage, series: sparklineFromBaseline(storage) },
  }
}

export function getApplicationsRequiringAttention(): Application[] {
  const applications = getDemoFixtures(mockApplications)
  return applications
    .filter((app) => ATTENTION_STATUSES.includes(app.status))
    .sort((a, b) => {
      const rank = (status: Status) =>
        status === 'failed' ? 0 : status === 'degraded' ? 1 : status === 'offline' ? 2 : 3
      return rank(a.status) - rank(b.status)
    })
}

export function getServerCapacity(): Server[] {
  const servers = getDemoFixtures(mockServers)
  return [...servers].sort(
    (a, b) =>
      Math.max(b.cpu ?? 0, b.memory ?? 0, b.disk ?? 0) -
      Math.max(a.cpu ?? 0, a.memory ?? 0, a.disk ?? 0),
  )
}

export function getRecentDeployments(limit = 8): Deployment[] {
  const deployments = getDemoFixtures(mockDeployments)
  return deployments.slice(0, limit)
}

export function getDashboardPanels() {
  return {
    backups: getDemoFixtures(mockBackupStatus),
    certificates: getDemoFixtures(mockCertificateWarnings),
    incidents: getDemoFixtures(mockIncidents),
    activity: getDemoFixtures(mockActivityFeed),
  }
}
