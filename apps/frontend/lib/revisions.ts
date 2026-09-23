import type { Revision, RevisionEnvVarMeta } from '@/lib/types'
import { allowSyntheticFallback } from '@/lib/mock-isolation'

export type RevisionFilterId = 'all' | 'active' | 'healthy' | 'archived'

export const REVISION_FILTERS: { id: RevisionFilterId; label: string }[] = [
  { id: 'all', label: 'All revisions' },
  { id: 'active', label: 'Active traffic' },
  { id: 'healthy', label: 'Healthy' },
  { id: 'archived', label: 'Archived' },
]

export function matchesRevisionFilter(rev: Revision, filter: RevisionFilterId): boolean {
  switch (filter) {
    case 'active':
      return !rev.archived && rev.traffic > 0
    case 'healthy':
      return !rev.archived && rev.status === 'healthy'
    case 'archived':
      return Boolean(rev.archived)
    case 'all':
    default:
      return true
  }
}

export function findRevision(revisionId: string, revisions: Revision[]): Revision | undefined {
  const direct = revisions.find(
    (r) => r.id === revisionId || r.number === revisionId || formatRevisionNumber(r.number) === revisionId,
  )
  if (direct) return direct

  // Dynamic fallback for valid UUID or rev-* identifier (only when demo mode is active)
  if (allowSyntheticFallback() && revisionId && (revisionId.startsWith('rev-') || revisionId.includes('-') || /^\d+$/.test(revisionId))) {
    const template = revisions[0]
    if (template) {
      const cleanNum = revisionId.replace(/^rev-/, '').slice(0, 5).padStart(5, '0')
      return {
        ...template,
        id: revisionId,
        number: cleanNum,
        commit: revisionId.slice(0, 7),
        commitMessage: `Revision snapshot for ${revisionId.slice(0, 8)}`,
        traffic: 0,
        archived: false,
      }
    }
  }

  return undefined
}

export function getRevisionsForApplication(
  applicationId: string,
  revisions: Revision[],
): Revision[] {
  return revisions
    .filter((r) => r.applicationId === applicationId)
    .sort((a, b) => Number(b.number) - Number(a.number))
}

/** Display revision number without leading zeros (e.g. 00049 → 49). */
export function formatRevisionNumber(number: string): string {
  const n = Number(number)
  return Number.isFinite(n) ? String(n) : number
}

export function getActiveRevision(revisions: Revision[]): Revision | undefined {
  const withTraffic = revisions
    .filter((r) => !r.archived && r.traffic > 0)
    .sort((a, b) => b.traffic - a.traffic)
  return withTraffic[0] ?? revisions.find((r) => r.status === 'healthy' && !r.archived)
}

export function buildRollbackPrompt(current: Revision, target: Revision): string {
  return `Rollback ${current.environment.toLowerCase()} application ${current.application} from revision ${formatRevisionNumber(current.number)} to revision ${formatRevisionNumber(target.number)}?`
}

export function canRollbackTo(current: Revision | undefined, target: Revision): boolean {
  if (!current) return false
  if (target.archived) return false
  if (current.id === target.id) return false
  if (current.applicationId !== target.applicationId) return false
  return Number(target.number) < Number(current.number)
}

export function compareFieldEqual(a: string, b: string): boolean {
  return a === b
}

export type RevisionCompareRow = {
  label: string
  left: string
  right: string
  changed: boolean
}

function listOrNone(items: string[]): string {
  return items.length ? items.join(', ') : '—'
}

function envMetaSummary(vars: RevisionEnvVarMeta[]): string {
  if (!vars.length) return '—'
  return vars
    .map((v) => `${v.key} (${v.scope}${v.secret ? ', secret ref' : ''})`)
    .join('\n')
}

function healthSummary(rev: Revision): string {
  return `${rev.healthCheck.type} ${rev.healthCheck.path} · every ${rev.healthCheck.interval}`
}

export function buildRevisionCompareRows(left: Revision, right: Revision): RevisionCompareRow[] {
  const rows: Omit<RevisionCompareRow, 'changed'>[] = [
    {
      label: 'Commit',
      left: `${left.commit} — ${left.commitMessage}`,
      right: `${right.commit} — ${right.commitMessage}`,
    },
    {
      label: 'Image',
      left: left.image,
      right: right.image,
    },
    {
      label: 'Image digest',
      left: left.imageDigest,
      right: right.imageDigest,
    },
    {
      label: 'CPU',
      left: `${left.cpuLimit} vCPU`,
      right: `${right.cpuLimit} vCPU`,
    },
    {
      label: 'RAM',
      left: `${left.memoryLimit} MB`,
      right: `${right.memoryLimit} MB`,
    },
    {
      label: 'Command',
      left: left.command,
      right: right.command,
    },
    {
      label: 'Environment variables',
      left: envMetaSummary(left.envVars),
      right: envMetaSummary(right.envVars),
    },
    {
      label: 'Secret references',
      left: listOrNone(left.secretRefs),
      right: listOrNone(right.secretRefs),
    },
    {
      label: 'Volumes',
      left: listOrNone(left.volumes),
      right: listOrNone(right.volumes),
    },
    {
      label: 'Health check',
      left: healthSummary(left),
      right: healthSummary(right),
    },
    {
      label: 'Domains',
      left: listOrNone(left.domains),
      right: listOrNone(right.domains),
    },
  ]

  return rows.map((row) => ({
    ...row,
    changed: row.left !== row.right,
  }))
}
