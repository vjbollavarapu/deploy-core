import type { EnvVarEntry } from '@/lib/types'

export type EnvVarKind = 'inherited' | 'overridden' | 'application-specific'

export const ENV_SCOPES = ['Organization', 'Project', 'Environment', 'Application'] as const

export const ENV_VAR_KIND_LABELS: Record<EnvVarKind, string> = {
  inherited: 'Inherited',
  overridden: 'Overridden',
  'application-specific': 'Application',
}

export function getEnvVarKind(entry: EnvVarEntry): EnvVarKind {
  if (entry.scope === 'Application') return 'application-specific'
  if (entry.overridden) return 'overridden'
  return 'inherited'
}

export function parseEnvText(text: string): { key: string; value: string; secret: boolean }[] {
  const rows: { key: string; value: string; secret: boolean }[] = []
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const cleaned = line.startsWith('export ') ? line.slice(7).trim() : line
    const eq = cleaned.indexOf('=')
    if (eq <= 0) continue
    const key = cleaned.slice(0, eq).trim()
    let value = cleaned.slice(eq + 1).trim()
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1)
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue
    rows.push({
      key,
      value,
      secret: /SECRET|PASSWORD|TOKEN|KEY|CREDENTIAL/i.test(key),
    })
  }
  return rows
}

export function filterEnvVars(
  variables: EnvVarEntry[],
  opts: { query?: string; scope?: string; kind?: EnvVarKind | 'all' },
): EnvVarEntry[] {
  const q = opts.query?.trim().toLowerCase() ?? ''
  return variables.filter((entry) => {
    if (opts.scope && opts.scope !== 'all' && entry.scope !== opts.scope) return false
    if (opts.kind && opts.kind !== 'all' && getEnvVarKind(entry) !== opts.kind) return false
    if (!q) return true
    return (
      entry.key.toLowerCase().includes(q) ||
      entry.source.toLowerCase().includes(q) ||
      (!entry.secret && entry.value.toLowerCase().includes(q))
    )
  })
}
