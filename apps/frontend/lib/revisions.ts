import type { Revision, RevisionEnvVarMeta } from '@/lib/types'

export function findRevision(revisionId: string, revisions: Revision[]): Revision | undefined {
  return revisions.find((r) => r.id === revisionId)
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
