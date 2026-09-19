import {
  applications,
  containerImages,
  containers,
  networks,
  volumes,
} from '@/lib/mock-data'
import { getServerLogLines } from '@/lib/observability'
import type {
  Application,
  Container,
  ContainerImage,
  DockerNetwork,
  LogLine,
  Server,
  Volume,
} from '@/lib/types'

export const SERVER_PROVIDERS = [
  'AWS',
  'Oracle Cloud',
  'Hetzner',
  'DigitalOcean',
  'GCP',
  'Azure',
  'Self-hosted',
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

export function findServer(serverId: string, servers: Server[]): Server | undefined {
  return servers.find((server) => server.id === serverId)
}

export function findServerByName(name: string, servers: Server[]): Server | undefined {
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

export function buildRegistrationCommand(token: string = REGISTRATION_TOKEN_PLACEHOLDER): string {
  return [
    'curl -fsSL https://get.deploycore.io/agent | sudo sh -s -- \\',
    `  --register-token ${token} \\`,
    '  --control-plane https://control.deploycore.io',
  ].join('\n')
}

export function isMaintenanceMode(server: Server): boolean {
  return server.status === 'maintenance'
}

/** Synthetic metric series for overview/metrics tabs. */
export function serverMetricSeries(server: Server, points = 24) {
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
