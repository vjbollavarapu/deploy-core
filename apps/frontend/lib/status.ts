import type { Status, StatusTone } from './types'

interface StatusConfig {
  label: string
  tone: StatusTone
}

/**
 * Central status → tone mapping. Pages must use StatusBadge / HealthIndicator /
 * getStatusConfig — never assign status colours ad hoc.
 */
export const STATUS_CONFIG: Record<Status, StatusConfig> = {
  healthy: { label: 'Healthy', tone: 'success' },
  running: { label: 'Running', tone: 'success' },
  deploying: { label: 'Deploying', tone: 'info' },
  pending: { label: 'Pending', tone: 'warning' },
  queued: { label: 'Queued', tone: 'warning' },
  stopped: { label: 'Stopped', tone: 'inactive' },
  degraded: { label: 'Degraded', tone: 'warning' },
  failed: { label: 'Failed', tone: 'critical' },
  offline: { label: 'Offline', tone: 'inactive' },
  maintenance: { label: 'Maintenance', tone: 'info' },
  cancelled: { label: 'Cancelled', tone: 'inactive' },
  unknown: { label: 'Unknown', tone: 'inactive' },
}

/** Canonical status identifiers (API / display aliases). */
export const STATUS = {
  HEALTHY: 'healthy',
  RUNNING: 'running',
  DEPLOYING: 'deploying',
  PENDING: 'pending',
  QUEUED: 'queued',
  STOPPED: 'stopped',
  DEGRADED: 'degraded',
  FAILED: 'failed',
  OFFLINE: 'offline',
  MAINTENANCE: 'maintenance',
  CANCELLED: 'cancelled',
  UNKNOWN: 'unknown',
} as const satisfies Record<string, Status>

export const TONE_CLASSES: Record<
  StatusTone,
  { dot: string; text: string; bg: string; border: string }
> = {
  success: {
    dot: 'bg-success',
    text: 'text-success',
    bg: 'bg-success/10',
    border: 'border-success/20',
  },
  warning: {
    dot: 'bg-warning',
    text: 'text-warning',
    bg: 'bg-warning/10',
    border: 'border-warning/20',
  },
  critical: {
    dot: 'bg-critical',
    text: 'text-critical',
    bg: 'bg-critical/10',
    border: 'border-critical/20',
  },
  info: {
    dot: 'bg-info',
    text: 'text-info',
    bg: 'bg-info/10',
    border: 'border-info/20',
  },
  inactive: {
    dot: 'bg-inactive',
    text: 'text-muted-foreground',
    bg: 'bg-muted',
    border: 'border-border',
  },
}

export function getStatusConfig(status: Status): StatusConfig {
  return STATUS_CONFIG[status] ?? STATUS_CONFIG.unknown
}

export function getToneClasses(tone: StatusTone) {
  return TONE_CLASSES[tone]
}
