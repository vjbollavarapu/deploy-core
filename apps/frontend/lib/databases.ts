import {
  backupJobs as rawBackupJobs,
  backupRuns as rawBackupRuns,
  databases as rawDatabases,
} from '@/lib/mock-data'
import { getDemoFixtures, allowSyntheticFallback } from '@/lib/mock-isolation'

const backupJobs = getDemoFixtures(rawBackupJobs)
const backupRuns = getDemoFixtures(rawBackupRuns)
const databases = getDemoFixtures(rawDatabases)
import { getDatabaseLogLines } from '@/lib/observability'
import type { BackupJob, BackupRun, DatabaseInstance, LogLine } from '@/lib/types'

export const DATABASE_SECTIONS = [
  { id: 'overview', label: 'Overview', suffix: '' },
  { id: 'connection', label: 'Connection', suffix: '/connection' },
  { id: 'metrics', label: 'Metrics', suffix: '/metrics' },
  { id: 'backups', label: 'Backups', suffix: '/backups' },
  { id: 'restore', label: 'Restore', suffix: '/restore' },
  { id: 'logs', label: 'Logs', suffix: '/logs' },
  { id: 'settings', label: 'Settings', suffix: '/settings' },
] as const

export type DatabaseSectionId = (typeof DATABASE_SECTIONS)[number]['id']

export const DATABASE_STATUS_FILTERS = [
  { label: 'Healthy', value: 'healthy' },
  { label: 'Degraded', value: 'degraded' },
  { label: 'Running', value: 'running' },
  { label: 'Failed', value: 'failed' },
] as const

export const DATABASE_ENGINE_FILTERS = [
  { label: 'PostgreSQL', value: 'PostgreSQL' },
  { label: 'MySQL', value: 'MySQL' },
  { label: 'Redis', value: 'Redis' },
  { label: 'MongoDB', value: 'MongoDB' },
] as const

export function findDatabase(
  databaseId: string,
  list: DatabaseInstance[] = databases,
): DatabaseInstance | undefined {
  const existing = list.find((db) => db.id === databaseId || db.name === databaseId)
  if (existing) return existing

  if (allowSyntheticFallback() && databaseId && databaseId !== 'undefined') {
    return {
      id: databaseId,
      name: databaseId.replace(/^db-/, ''),
      type: 'PostgreSQL',
      version: '16',
      project: 'Daya Platform',
      environment: 'Production',
      server: 'prod-edge-01',
      storageUsedGb: 12,
      storageTotalGb: 50,
      backups: 4,
      lastBackup: '2 hours ago',
      status: 'healthy',
      dbName: 'deploycore_db',
      port: 5432,
      username: 'postgres',
      connectionHost: 'postgres.internal.daya.io',
      credentialsRevealAllowed: true,
    }
  }
  return undefined
}

/** Demo fixtures only — production must load policy/runs from the Control Plane. */
export function getDatabaseBackupJob(database: DatabaseInstance): BackupJob | undefined {
  return backupJobs.find(
    (job) => job.databaseId === database.id || job.database === database.name,
  )
}

/** Demo fixtures only — never synthesize successful backups in production. */
export function getDatabaseBackupRuns(database: DatabaseInstance): BackupRun[] {
  return backupRuns
    .filter((run) => run.databaseId === database.id || run.database === database.name)
    .sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1))
}

/** Wire Control Plane backup list/create payloads into the BackupRun presentation model. */
export type WireBackup = {
  id: string
  resourceId?: string
  status?: string
  startedAt?: string | null
  completedAt?: string | null
  durationMs?: number | null
  sizeBytes?: number | null
  checksum?: string
  destinationUri?: string
  destinationType?: string
  jobId?: string | null
  createdAt?: string
}

export function mapWireBackupToRun(
  backup: WireBackup,
  database: Pick<DatabaseInstance, 'id' | 'name'>,
): BackupRun {
  const status = mapBackupStatus(backup.status)
  const sizeBytes = backup.sizeBytes ?? 0
  const sizeGb = sizeBytes > 0 ? Math.round((sizeBytes / (1024 ** 3)) * 1000) / 1000 : 0
  return {
    id: backup.id,
    backupJobId: backup.jobId ?? '',
    database: database.name,
    databaseId: database.id,
    status,
    startedAt: backup.startedAt || backup.createdAt || '—',
    completedAt: backup.completedAt || '—',
    duration: formatDurationMs(backup.durationMs),
    sizeGb,
    destination: backup.destinationUri || backup.destinationType || '—',
    checksum: backup.checksum || '—',
  }
}

function mapBackupStatus(raw?: string): BackupRun['status'] {
  switch ((raw || '').toUpperCase()) {
    case 'SUCCEEDED':
      return 'success'
    case 'FAILED':
    case 'EXPIRED':
    case 'DELETED':
      return 'failed'
    case 'RUNNING':
      return 'running'
    case 'QUEUED':
    case 'PENDING':
      return 'queued'
    default:
      return 'running'
  }
}

function formatDurationMs(ms?: number | null): string {
  if (ms == null || ms < 0) return '—'
  if (ms < 1000) return `${ms}ms`
  const sec = Math.round(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  const rem = sec % 60
  return rem ? `${min}m ${rem}s` : `${min}m`
}

export function getDatabaseLogs(database: DatabaseInstance, count = 80): LogLine[] {
  return getDatabaseLogLines(database, count)
}

export function buildConnectionUrl(database: DatabaseInstance, password = '••••••••'): string {
  const scheme =
    database.type === 'PostgreSQL'
      ? 'postgresql'
      : database.type === 'MySQL'
        ? 'mysql'
        : database.type === 'Redis'
          ? 'redis'
          : 'mongodb'
  if (database.type === 'Redis') {
    return `${scheme}://:${password}@${database.connectionHost}:${database.port}`
  }
  return `${scheme}://${database.username}:${password}@${database.connectionHost}:${database.port}/${database.dbName}`
}

/** Synthetic metric samples for database overview/metrics. */
export function databaseMetricSeries(database: DatabaseInstance, points = 24) {
  const storagePct = Math.round((database.storageUsedGb / database.storageTotalGb) * 100)
  const baseCpu = database.status === 'degraded' ? 78 : 32
  const baseConn = database.status === 'degraded' ? 140 : 48
  return Array.from({ length: points }).map((_, index) => {
    const wave = Math.sin(index / 3) * 8
    return {
      t: `${points - index}m`,
      cpu: Math.min(100, Math.max(0, Math.round(baseCpu + wave))),
      connections: Math.max(0, Math.round(baseConn + wave * 2)),
      storage: storagePct,
    }
  })
}
