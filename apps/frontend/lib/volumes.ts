import type { Status, Volume } from '@/lib/types'

export interface WireVolume {
  id: string
  organizationId?: string
  serverId?: string
  name: string
  driver: string
  mountPath: string
  state?: string
  attachedResourceType?: string | null
  attachedResourceId?: string | null
  backupPolicy?: Record<string, unknown>
  protected?: boolean
  dockerName?: string | null
  usageBytes?: number | null
  labels?: Record<string, unknown>
  lastError?: string
  createdAt?: string
  updatedAt?: string
}

export function mapVolumeStateToStatus(state?: string): Status {
  switch (state?.toUpperCase()) {
    case 'ATTACHED':
    case 'READY':
      return 'healthy'
    case 'CREATING':
    case 'PENDING':
      return 'deploying'
    case 'DETACHING':
    case 'DELETING':
      return 'degraded'
    case 'DELETED':
      return 'stopped'
    case 'FAILED':
      return 'failed'
    default:
      return 'healthy'
  }
}

export function wireVolumeToViewModel(
  wire: WireVolume,
  fallback?: Partial<Volume>,
  serverName?: string,
): Volume {
  const usageGb = wire.usageBytes ? Math.round(wire.usageBytes / (1024 * 1024 * 1024)) : fallback?.usedGb ?? 10
  const attached = wire.attachedResourceType && wire.attachedResourceId
    ? `${wire.attachedResourceType}/${wire.attachedResourceId.slice(0, 8)}`
    : fallback?.attachedResource ?? 'Unattached'

  let policyStr = 'Daily @ 02:00 UTC'
  if (wire.backupPolicy && typeof wire.backupPolicy === 'object') {
    if (typeof wire.backupPolicy.schedule === 'string') {
      policyStr = wire.backupPolicy.schedule
    } else if (wire.backupPolicy.enabled === false) {
      policyStr = 'Disabled'
    }
  } else if (fallback?.backupPolicy) {
    policyStr = fallback.backupPolicy
  }

  return {
    id: wire.id || fallback?.id || 'vol-unknown',
    name: wire.name || fallback?.name || 'volume',
    server: serverName || fallback?.server || 'srv-hetzner-fsn1-01',
    driver: wire.driver || fallback?.driver || 'local',
    attachedResource: attached,
    mountPath: wire.mountPath || fallback?.mountPath || '/data',
    backupPolicy: policyStr,
    status: mapVolumeStateToStatus(wire.state) || fallback?.status || 'healthy',
    usedGb: usageGb,
    totalGb: fallback?.totalGb ?? 50,
    mounts: fallback?.mounts ?? 1,
  }
}
