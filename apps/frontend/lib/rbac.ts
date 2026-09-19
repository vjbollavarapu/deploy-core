import type { AuditLogEntry, PermissionLevel } from '@/lib/types'

export const DEFAULT_ROLES = [
  {
    name: 'Owner',
    description: 'Full control of the organization, billing, and membership.',
  },
  {
    name: 'Administrator',
    description: 'Manage infrastructure and applications; limited org settings.',
  },
  {
    name: 'DevOps',
    description: 'Operate servers, deployments, databases, and backups.',
  },
  {
    name: 'Developer',
    description: 'Deploy and configure applications within assigned projects.',
  },
  {
    name: 'Support',
    description: 'Read-mostly access for troubleshooting production issues.',
  },
  {
    name: 'Viewer',
    description: 'Read-only visibility into projects and runtime status.',
  },
] as const

const SECRET_KEY_PATTERN = /secret|password|token|apikey|api_key|private|credential/i

/** Strip or mask sensitive audit metadata so secret values never render. */
export function sanitizeAuditMetadata(
  metadata: Record<string, string> | undefined,
  action = '',
): Record<string, string> | undefined {
  if (!metadata) return undefined
  const secretAction = /secret/i.test(action)
  return Object.fromEntries(
    Object.entries(metadata).map(([key, value]) => {
      if (SECRET_KEY_PATTERN.test(key) || (secretAction && /^(value|after|before)$/i.test(key))) {
        return [key, '••••••••']
      }
      return [key, value]
    }),
  )
}

export function sanitizeAuditEntry(entry: AuditLogEntry): AuditLogEntry {
  return {
    ...entry,
    before: sanitizeAuditMetadata(entry.before, entry.action),
    after: sanitizeAuditMetadata(entry.after, entry.action),
  }
}

export function permissionLevelLabel(level: PermissionLevel): string {
  return level
}
