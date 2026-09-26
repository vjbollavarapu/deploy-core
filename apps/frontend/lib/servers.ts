import {
  applications as rawApplications,
  containerImages as rawContainerImages,
  containers as rawContainers,
  networks as rawNetworks,
  servers as rawMockServers,
  volumes as rawVolumes,
} from '@/lib/mock-data'
import { getDemoFixtures, allowSyntheticFallback } from '@/lib/mock-isolation'
import { getServerLogLines } from '@/lib/observability'
import type { Server as WireServer } from '@/lib/api'
import type {
  Application,
  Container,
  ContainerImage,
  DockerNetwork,
  LogLine,
  Server,
  Status,
  Volume,
} from '@/lib/types'

const applications = getDemoFixtures(rawApplications)
const containerImages = getDemoFixtures(rawContainerImages)
const containers = getDemoFixtures(rawContainers)
const networks = getDemoFixtures(rawNetworks)
const mockServers = getDemoFixtures(rawMockServers)
const volumes = getDemoFixtures(rawVolumes)

export const SERVER_PROVIDERS = [
  'AWS',
  'Oracle Cloud',
  'Hetzner',
  'DigitalOcean',
  'GCP',
  'Azure',
  'Self-hosted',
] as const

export const SERVER_STATUS_FILTERS = [
  { value: 'all', label: 'All statuses' },
  { value: 'running', label: 'Running' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'maintenance', label: 'Maintenance' },
  { value: 'offline', label: 'Offline' },
] as const

export const SERVER_SECTIONS = [
  { id: 'overview', label: 'Overview', suffix: '' },
  { id: 'applications', label: 'Applications', suffix: '/applications' },
  { id: 'containers', label: 'Containers', suffix: '/containers' },
  { id: 'images', label: 'Images', suffix: '/images' },
  { id: 'volumes', label: 'Volumes', suffix: '/volumes' },
  { id: 'networks', label: 'Networks', suffix: '/networks' },
  { id: 'metrics', label: 'Metrics', suffix: '/metrics' },
  { id: 'logs', label: 'Logs', suffix: '/logs' },
  { id: 'agent', label: 'Agent', suffix: '/agent' },
  { id: 'settings', label: 'Settings', suffix: '/settings' },
] as const

export type ServerSectionId = (typeof SERVER_SECTIONS)[number]['id']

/** Deterministic placeholder registration token for UI demos. */
export const REGISTRATION_TOKEN_PLACEHOLDER = 'dc_reg_tmp_8f3a2c1e9b7d4a60'

/** Neutral display for values the Control Plane has not reported. */
export const UNAVAILABLE = '—'

export function displayServerValue(
  value: string | number | null | undefined,
  empty: string = UNAVAILABLE,
): string {
  if (value === null || value === undefined || value === '') return empty
  return String(value)
}

/**
 * Initial registration incomplete: no successful agent heartbeat has ever been
 * recorded. A previously registered agent that is temporarily offline retains
 * lastHeartbeatAt from its last heartbeat — do not treat that as first-boot recovery.
 */
export function canIssueInitialRegistrationToken(server: Server): boolean {
  return server.lastHeartbeatAt == null && server.status === 'offline'
}

function bytesToGb(bytes: number | null | undefined): number | null {
  if (bytes == null || !Number.isFinite(bytes) || bytes <= 0) return null
  return Math.round((bytes / (1024 * 1024 * 1024)) * 10) / 10
}

function formatHeartbeatRelative(iso: string): string {
  try {
    const diffMs = Date.now() - new Date(iso).getTime()
    if (Number.isNaN(diffMs)) return iso
    if (diffMs < 60000) {
      return `${Math.max(1, Math.round(diffMs / 1000))}s ago`
    }
    if (diffMs < 3600000) {
      return `${Math.round(diffMs / 60000)}m ago`
    }
    if (diffMs < 86400000) {
      return `${Math.round(diffMs / 3600000)}h ago`
    }
    return `${Math.round(diffMs / 86400000)}d ago`
  } catch {
    return iso
  }
}

export function findServer(serverId: string, servers: Server[] = mockServers): Server | undefined {
  const found = servers.find(
    (server) => server.id === serverId || server.name.toLowerCase() === serverId.toLowerCase(),
  )
  if (found) return found
  // Demo-only synthetic host for offline preview of unknown IDs.
  if (allowSyntheticFallback() && serverId && serverId !== 'undefined') {
    return {
      id: serverId,
      name: serverId.startsWith('srv-') ? serverId : `srv-${serverId.slice(0, 8)}`,
      provider: 'Hetzner',
      region: 'eu-central-1',
      ip: '192.0.2.10',
      privateIp: '10.0.0.10',
      cpu: 18,
      cpuCores: 4,
      memory: 34,
      memoryTotalGb: 16,
      disk: 42,
      diskTotalGb: 160,
      containers: 2,
      agentVersion: 'v1.4.2',
      status: 'running',
      lastHeartbeat: '10s ago',
      lastHeartbeatAt: new Date().toISOString(),
      os: 'Ubuntu 24.04 LTS',
      arch: 'x86_64',
      dockerVersion: '27.1.1',
      uptime: '14d 6h',
      load: [0.42, 0.38, 0.35],
    }
  }
  return undefined
}

export function wireServerToViewModel(
  wire: WireServer,
  fallback?: Partial<Server>,
): Server {
  const isMaint = Boolean(wire.maintenanceMode || wire.status === 'MAINTENANCE')
  let mappedStatus: Status = 'offline'
  if (isMaint) {
    mappedStatus = 'maintenance'
  } else if (wire.status === 'ONLINE') {
    mappedStatus = 'running'
  } else if (wire.status === 'DEGRADED') {
    mappedStatus = 'degraded'
  } else if (wire.status === 'OFFLINE' || wire.status === 'DISABLED') {
    mappedStatus = 'offline'
  } else if (wire.status) {
    mappedStatus = 'unknown'
  }

  const lastHeartbeatAt = wire.lastHeartbeatAt ?? null
  const lastHeartbeat = lastHeartbeatAt ? formatHeartbeatRelative(lastHeartbeatAt) : null

  const id = wire.id || fallback?.id || 'srv-unknown'
  const name = wire.name || fallback?.name || (wire.hostname ?? id)

  const labelAgent =
    wire.labels && typeof wire.labels.agentVersion === 'string'
      ? wire.labels.agentVersion
      : null

  return {
    id,
    name,
    provider: wire.provider || fallback?.provider || 'Unknown',
    region: wire.region?.trim() ? wire.region : (fallback?.region ?? null),
    ip: wire.publicIp?.trim() ? wire.publicIp : (fallback?.ip ?? null),
    privateIp: wire.privateIp?.trim() ? wire.privateIp : (fallback?.privateIp ?? null),
    // Live utilization is not on the server list/detail contract — never invent it.
    cpu: fallback?.cpu ?? null,
    cpuCores: wire.cpuCores ?? fallback?.cpuCores ?? null,
    memory: fallback?.memory ?? null,
    memoryTotalGb: bytesToGb(wire.memoryBytes) ?? fallback?.memoryTotalGb ?? null,
    disk: fallback?.disk ?? null,
    diskTotalGb: bytesToGb(wire.diskBytes) ?? fallback?.diskTotalGb ?? null,
    containers: fallback?.containers ?? null,
    agentVersion: labelAgent ?? fallback?.agentVersion ?? null,
    status: mappedStatus,
    lastHeartbeat,
    lastHeartbeatAt,
    os: wire.operatingSystem?.trim()
      ? wire.operatingSystem
      : (fallback?.os ?? null),
    arch: wire.architecture?.trim() ? wire.architecture : (fallback?.arch ?? null),
    dockerVersion: wire.dockerVersion?.trim()
      ? wire.dockerVersion
      : (fallback?.dockerVersion ?? null),
    uptime: fallback?.uptime ?? null,
    load: fallback?.load ?? null,
  }
}

export function findServerByName(name: string, servers: Server[] = mockServers): Server | undefined {
  return servers.find((server) => server.name === name)
}

export function getServerApplications(server: Server): Application[] {
  return applications.filter((app) => app.server === server.name)
}

export function getServerContainers(server: Server): Container[] {
  return containers.filter((container) => container.server === server.name)
}

export function getServerVolumes(server: Server): Volume[] {
  return volumes.filter((volume) => volume.server === server.name)
}

export function getServerImages(server: Server): ContainerImage[] {
  const appNames = new Set(getServerApplications(server).map((app) => app.name))
  const fromContainers = new Set(
    getServerContainers(server).map((container) => container.image.split(':')[0]),
  )
  return containerImages.filter(
    (image) => appNames.has(image.application) || fromContainers.has(image.name),
  )
}

export function getServerNetworks(server: Server): DockerNetwork[] {
  const appNames = new Set(getServerApplications(server).map((app) => app.name))
  return networks.filter((network) =>
    network.connectedServices.some((service) => appNames.has(service)),
  )
}

export function getServerLogs(server: Server, count = 80): LogLine[] {
  return getServerLogLines(server, count)
}

export function buildRegistrationCommand(
  token: string = REGISTRATION_TOKEN_PLACEHOLDER,
  serverId: string = '<SERVER_UUID>',
  serverUrl: string = 'https://control.deploycore.io',
): string {
  return [
    '# Obtain install-agent.sh from the DeployCore release (verify SHA-256; do not curl|bash).',
    'sudo ./install-agent.sh \\',
    '  --install-docker \\',
    `  --server-url ${serverUrl} \\`,
    `  --token ${token} \\`,
    `  --server-id ${serverId} \\`,
    '  --binary ./deploycore-agent-linux-amd64 \\',
    '  --checksum ./SHA256SUMS \\',
    '  --acme-email ops@example.com',
  ].join('\n')
}

export function isMaintenanceMode(server: Server): boolean {
  return server.status === 'maintenance'
}

/** Metric series only when live utilization samples exist; otherwise empty. */
export function serverMetricSeries(server: Server, points = 24) {
  if (server.cpu == null || server.memory == null || server.disk == null) {
    return [] as Array<{ t: string; cpu: number; memory: number; disk: number }>
  }
  const baseCpu = server.cpu
  const baseMem = server.memory
  const baseDisk = server.disk
  return Array.from({ length: points }).map((_, index) => {
    const wave = Math.sin(index / 3) * 6
    return {
      t: `${points - index}m`,
      cpu: Math.min(100, Math.max(0, Math.round(baseCpu + wave + (index % 5) - 2))),
      memory: Math.min(100, Math.max(0, Math.round(baseMem + wave * 0.6))),
      disk: Math.min(100, Math.max(0, Math.round(baseDisk + (index % 3) - 1))),
    }
  })
}
