import {
  backupJobs,
  backupRuns,
  databases,
} from '@/lib/mock-data'
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

export function findDatabase(
  databaseId: string,
  list: DatabaseInstance[] = databases,
): DatabaseInstance | undefined {
  return list.find((db) => db.id === databaseId)
}

export function getDatabaseBackupJob(database: DatabaseInstance): BackupJob | undefined {
  return backupJobs.find((job) => job.databaseId === database.id || job.database === database.name)
}

export function getDatabaseBackupRuns(database: DatabaseInstance): BackupRun[] {
  return backupRuns
    .filter((run) => run.databaseId === database.id || run.database === database.name)
    .sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1))
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
