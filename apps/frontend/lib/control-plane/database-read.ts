/** Production database list, create, and detail mapping. No credential plaintext. */

export interface DatabaseReadClient {
  get<T>(path: string): Promise<T>
  post<T>(path: string, body?: unknown): Promise<T>
}

export type DatabaseLoadResult<T> =
  | { kind: 'ok'; value: T }
  | { kind: 'not-found' }
  | { kind: 'error'; message: string }

export interface WireDatabase {
  id?: string
  organizationId?: string
  projectId?: string
  environmentId?: string
  serverId?: string
  name?: string
  engine?: string
  engineVersion?: string
  databaseName?: string
  username?: string
  storageVolumeName?: string
  status?: string
  hasCredential?: boolean
  lastError?: string
  privateHost?: string
  port?: number
  password?: string
}

export interface ProductionDatabase {
  id: string
  name: string
  type: 'PostgreSQL' | 'MySQL' | 'Redis' | 'MongoDB'
  version: string
  project: string
  environment: string
  server: string
  storageUsedGb: number
  storageTotalGb: number
  storageVolumeName?: string
  backups: number
  lastBackup: string
  status: 'running' | 'degraded' | 'failed' | 'stopped' | 'pending' | 'unknown'
  dbName: string
  port: number
  username: string
  connectionHost: string
  credentialsRevealAllowed: boolean
}

export interface DatabaseNameLookup {
  projectName?: string | null
  environmentName?: string | null
  serverName?: string | null
}

export interface ProvisionDatabaseInput {
  organizationId: string
  projectId: string
  environmentId: string
  serverId: string
  name: string
  engineVersion: string
  databaseName: string
  username: string
  storageVolume?: string
}

const ENVIRONMENTS_COLLECTION = /^\/environments(?:\?|$)/

export function databaseListPath(organizationId: string): string {
  return `/databases?organizationId=${encodeURIComponent(organizationId)}`
}

export function projectListPath(organizationId: string): string {
  return `/projects?organizationId=${encodeURIComponent(organizationId)}`
}

export function serverListPath(organizationId: string): string {
  return `/servers?organizationId=${encodeURIComponent(organizationId)}`
}

export function projectEnvironmentsPath(projectId: string): string {
  return `/projects/${encodeURIComponent(projectId)}/environments`
}

export function databaseDetailPath(databaseId: string): string {
  return `/databases/${encodeURIComponent(databaseId)}`
}

export function isEnvironmentsCollectionPath(path: string): boolean {
  return ENVIRONMENTS_COLLECTION.test(path)
}

export function databaseCreateBody(input: ProvisionDatabaseInput) {
  const body: Record<string, string> = {
    organizationId: input.organizationId,
    projectId: input.projectId,
    environmentId: input.environmentId,
    serverId: input.serverId,
    name: input.name.trim(),
    engine: 'postgresql',
    engineVersion: input.engineVersion.trim() || '16',
    databaseName: input.databaseName.trim(),
    username: input.username.trim(),
  }
  const volume = input.storageVolume?.trim()
  if (volume) body.storageVolume = volume
  return body
}

export function mapDatabaseEngine(raw?: string): ProductionDatabase['type'] {
  switch ((raw ?? '').toLowerCase()) {
    case 'mysql':
      return 'MySQL'
    case 'redis':
      return 'Redis'
    case 'mongo':
    case 'mongodb':
      return 'MongoDB'
    default:
      return 'PostgreSQL'
  }
}

export function mapDatabaseStatus(raw?: string): ProductionDatabase['status'] {
  switch ((raw ?? '').toUpperCase()) {
    case 'RUNNING':
      return 'running'
    case 'DEGRADED':
      return 'degraded'
    case 'FAILED':
      return 'failed'
    case 'STOPPED':
    case 'DELETED':
      return 'stopped'
    case 'PENDING':
    case 'PROVISIONING':
    case 'DELETING':
      return 'pending'
    default:
      return 'unknown'
  }
}

function displayLabel(name?: string | null, id?: string | null): string {
  const named = name?.trim()
  if (named) return named
  const ident = id?.trim()
  if (ident) return ident
  return '—'
}

export function mapWireDatabase(wire: WireDatabase, names?: DatabaseNameLookup): ProductionDatabase {
  const engine = mapDatabaseEngine(wire.engine)
  return {
    id: wire.id ?? '',
    name: displayLabel(wire.name, wire.id),
    type: engine,
    version: wire.engineVersion?.trim() || '—',
    project: displayLabel(names?.projectName, wire.projectId),
    environment: displayLabel(names?.environmentName, wire.environmentId),
    server: displayLabel(names?.serverName, wire.serverId),
    storageUsedGb: 0,
    storageTotalGb: 0,
    storageVolumeName: wire.storageVolumeName?.trim() || undefined,
    backups: 0,
    lastBackup: '—',
    status: mapDatabaseStatus(wire.status),
    dbName: wire.databaseName?.trim() || '—',
    port: typeof wire.port === 'number' && wire.port > 0 ? wire.port : 0,
    username: wire.username?.trim() || '—',
    connectionHost: displayLabel(wire.privateHost, undefined),
    credentialsRevealAllowed: wire.hasCredential === true,
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) return err.message
  return 'Unable to reach the control plane'
}

function isNotFound(err: unknown): boolean {
  return typeof err === 'object' && err !== null && 'status' in err && (err as { status?: number }).status === 404
}

function indexNames(items: Array<{ id?: string; name?: string }> | undefined): Map<string, string> {
  const names = new Map<string, string>()
  if (!Array.isArray(items)) return names
  for (const item of items) {
    if (item.id && item.name?.trim()) names.set(item.id, item.name.trim())
  }
  return names
}

export async function loadProductionDatabaseList(
  client: DatabaseReadClient,
  organizationId: string,
): Promise<DatabaseLoadResult<ProductionDatabase[]>> {
  let items: WireDatabase[]
  try {
    const body = await client.get<{ items?: WireDatabase[] }>(databaseListPath(organizationId))
    items = Array.isArray(body.items) ? body.items : []
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }

  const [projects, servers] = await Promise.all([
    client.get<{ items?: Array<{ id?: string; name?: string }> }>(projectListPath(organizationId)).catch(() => null),
    client.get<{ items?: Array<{ id?: string; name?: string }> }>(serverListPath(organizationId)).catch(() => null),
  ])
  const projectNames = indexNames(projects?.items)
  const serverNames = indexNames(servers?.items)

  return {
    kind: 'ok',
    value: items.map((item) =>
      mapWireDatabase(item, {
        projectName: item.projectId ? projectNames.get(item.projectId) : null,
        serverName: item.serverId ? serverNames.get(item.serverId) : null,
      }),
    ),
  }
}

export async function createProductionDatabase(
  client: DatabaseReadClient,
  input: ProvisionDatabaseInput,
): Promise<{ id: string }> {
  const response = await client.post<{ database?: WireDatabase }>('/databases', databaseCreateBody(input))
  return { id: response.database?.id ?? '' }
}

async function readResourceName(
  client: DatabaseReadClient,
  path: string,
  pick: (body: Record<string, unknown>) => unknown,
): Promise<string | null> {
  try {
    const body = await client.get<Record<string, unknown>>(path)
    const value = pick(body)
    return typeof value === 'string' && value.trim() ? value.trim() : null
  } catch {
    return null
  }
}

export async function loadProductionDatabase(
  client: DatabaseReadClient,
  databaseId: string,
): Promise<DatabaseLoadResult<ProductionDatabase>> {
  let wire: WireDatabase
  try {
    const body = await client.get<{ database?: WireDatabase }>(databaseDetailPath(databaseId))
    if (!body.database?.id) return { kind: 'not-found' }
    wire = body.database
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }

  const [projectName, environmentName, serverName] = await Promise.all([
    wire.projectId
      ? readResourceName(client, `/projects/${wire.projectId}`, (body) => (body.project as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
    wire.environmentId
      ? readResourceName(
          client,
          `/environments/${wire.environmentId}`,
          (body) => (body.environment as { name?: string } | undefined)?.name,
        )
      : Promise.resolve(null),
    wire.serverId
      ? readResourceName(client, `/servers/${wire.serverId}`, (body) => (body.server as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
  ])

  return {
    kind: 'ok',
    value: mapWireDatabase(wire, { projectName, environmentName, serverName }),
  }
}
