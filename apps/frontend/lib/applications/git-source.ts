import type { StatusTone } from '../types'
import {
  authModeLabel,
  collectRepositoryPages,
  connectionStatusView,
  REPOSITORY_PAGE_LIMIT,
  safeRepositoryHref,
  type RepositoryPage,
} from '../github/providers'

/** Same UUID shape the application create wizard already accepts. */
const CONNECTION_ID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

export const DEFAULT_GIT_BRANCH = 'main'

export interface GitSourceConnection {
  id: string
  organizationId?: string
  /** Wire provider, such as github. */
  provider?: string
  /** Mapped provider label, such as GitHub, when the wire provider is absent. */
  type?: string
  authMode?: string
  /** Backend status: active, error, disabled, or revoked. */
  providerStatus?: string
  /** Used when providerStatus is omitted and the value is a backend status. */
  status?: string
}

export interface GitSourceRepository {
  id: string
  connectionId: string
  organizationId: string
  cloneUrl: string
  defaultBranch?: string | null
}

export interface ConnectionEligibility {
  selectable: boolean
  visibility: 'selectable' | 'unavailable' | 'hidden'
  statusLabel: string
  statusTone: StatusTone
  authModeLabel: string | null
}

export interface ClassifiedGitConnection<T extends GitSourceConnection> {
  connection: T
  eligibility: ConnectionEligibility
}

export type ConnectedRepositoryResult =
  | { ok: true; repositoryId: string; repositoryUrl: string; branch: string }
  | { ok: false; code: ConnectedGitRejection; message: string }

export type ConnectedGitRejection =
  | 'organization'
  | 'provider'
  | 'auth_mode'
  | 'status'
  | 'repository_missing'
  | 'repository_connection'
  | 'repository_organization'
  | 'clone_url'
  | 'connection_id'

export interface ConnectedSourceFields {
  gitConnectionId: string
  repositoryId: string
  repositoryUrl: string
  branch: string
}

export type GitSourceCreateInput =
  | { repositorySource: 'public' }
  | {
      repositorySource: 'connected'
      organizationId: string
      connection: GitSourceConnection
      repository?: GitSourceRepository | null
    }

export type GitSourceCreateResult<TConfig> =
  | { ok: true; config: TConfig }
  | { ok: false; code: ConnectedGitRejection; message: string }

const CLONE_URL_CREDENTIAL_QUERY_KEYS = new Set([
  'token',
  'access_token',
  'auth',
  'authorization',
  'password',
  'passwd',
  'credential',
  'key',
  'api_key',
  'apikey',
])

export function isDeploymentConnectionId(value: string | null | undefined): value is string {
  return Boolean(value && CONNECTION_ID_PATTERN.test(value))
}

/** Clone URL check for connected Git. Display links still use safeRepositoryHref. */
export function credentialFreeCloneUrl(url: string | null | undefined): string | null {
  if (!url) return null
  const href = safeRepositoryHref(url)
  if (!href) return null
  let parsed: URL
  try {
    parsed = new URL(href)
  } catch {
    return null
  }
  if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return null
  if (!isCloneHostname(parsed.hostname)) return null
  if (parsed.username || parsed.password) return null
  for (const name of parsed.searchParams.keys()) {
    if (CLONE_URL_CREDENTIAL_QUERY_KEYS.has(name.toLowerCase())) return null
  }
  return href
}

/** Credential query names, including a schemeless repository string such as host/path?token=secret. */
export function repositoryHasCredentialQuery(value: string | null | undefined): boolean {
  const trimmed = value?.trim() ?? ''
  if (!trimmed) return false
  if (urlHasCredentialQuery(trimmed)) return true
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return false
  return urlHasCredentialQuery(`https://${trimmed}`) || queryTextHasCredentialName(trimmed)
}

function urlHasCredentialQuery(value: string): boolean {
  try {
    const parsed = new URL(value)
    for (const name of parsed.searchParams.keys()) {
      if (CLONE_URL_CREDENTIAL_QUERY_KEYS.has(name.toLowerCase())) return true
    }
  } catch {
    return queryTextHasCredentialName(value)
  }
  return false
}

function queryTextHasCredentialName(value: string): boolean {
  const start = value.indexOf('?')
  if (start < 0) return false
  const query = value.slice(start + 1).split('#')[0]
  const params = new URLSearchParams(query)
  for (const name of params.keys()) {
    if (CLONE_URL_CREDENTIAL_QUERY_KEYS.has(name.toLowerCase())) return true
  }
  return false
}

export function gitConnectionListRequest(
  organizationId: string,
  page?: { limit?: number; offset?: number },
): { path: string } {
  const params = new URLSearchParams()
  params.set('organizationId', organizationId)
  if (page?.limit != null) params.set('limit', String(page.limit))
  if (page?.offset != null) params.set('offset', String(page.offset))
  return { path: `/integrations/git/connections?${params.toString()}` }
}

export async function collectGitConnectionPages<T>(
  organizationId: string,
  fetchPage: (request: {
    organizationId: string
    limit: number
    offset: number
  }) => Promise<RepositoryPage<T>>,
): Promise<T[]> {
  return collectRepositoryPages((page) =>
    fetchPage({
      organizationId,
      limit: page.limit,
      offset: page.offset,
    }),
  )
}

export function gitConnectionEligibility(
  connection: GitSourceConnection,
  organizationId: string,
): ConnectionEligibility {
  const providerStatus = backendStatus(connection)
  const status = connectionStatusView(providerStatus || undefined, 'unknown')
  const modeLabel = authModeLabel(connection.authMode)
  const sameOrganization = connection.organizationId === organizationId && organizationId.length > 0
  const github = isGitHubProvider(connection)
  const supportedMode = connection.authMode === 'github_app' || connection.authMode === 'pat'
  const active = providerStatus === 'active'
  const selectable = sameOrganization && github && supportedMode && active
  let visibility: ConnectionEligibility['visibility'] = 'hidden'
  if (selectable) visibility = 'selectable'
  else if (sameOrganization && github) visibility = 'unavailable'
  return {
    selectable,
    visibility,
    statusLabel: status.label,
    statusTone: status.tone,
    authModeLabel: modeLabel,
  }
}

export function classifyGitConnections<T extends GitSourceConnection>(
  connections: readonly T[],
  organizationId: string,
): ClassifiedGitConnection<T>[] {
  const ranked = connections.map((connection, index) => ({
    connection,
    eligibility: gitConnectionEligibility(connection, organizationId),
    index,
  }))
  ranked.sort((left, right) => {
    const difference = selectionRank(left) - selectionRank(right)
    if (difference !== 0) return difference
    return left.index - right.index
  })
  return ranked.map(({ connection, eligibility }) => ({ connection, eligibility }))
}

export function selectConnectedRepository(input: {
  organizationId: string
  connection: GitSourceConnection
  repository: GitSourceRepository
}): ConnectedRepositoryResult {
  const eligibility = gitConnectionEligibility(input.connection, input.organizationId)
  if (!eligibility.selectable) {
    return { ok: false, ...rejectionForConnection(input.connection, input.organizationId) }
  }
  if (!isDeploymentConnectionId(input.connection.id)) {
    return { ok: false, code: 'connection_id', message: 'Select a GitHub connection.' }
  }
  if (!input.repository?.id) {
    return { ok: false, code: 'repository_missing', message: 'Choose a repository.' }
  }
  if (input.repository.connectionId !== input.connection.id) {
    return { ok: false, code: 'repository_connection', message: 'Choose a repository from the selected connection.' }
  }
  if (input.repository.organizationId !== input.organizationId) {
    return { ok: false, code: 'repository_organization', message: 'Choose a repository from this organization.' }
  }
  const repositoryUrl = credentialFreeCloneUrl(input.repository.cloneUrl)
  if (!repositoryUrl) {
    return { ok: false, code: 'clone_url', message: 'Repository URL must be a credential-free http(s) URL.' }
  }
  return {
    ok: true,
    repositoryId: input.repository.id,
    repositoryUrl,
    branch: branchFromDefault(input.repository.defaultBranch),
  }
}

export function branchFromDefault(defaultBranch: string | null | undefined): string {
  const trimmed = defaultBranch?.trim() ?? ''
  return trimmed.length > 0 ? trimmed : DEFAULT_GIT_BRANCH
}

export function fieldsAfterRepositorySelection<T extends ConnectedSourceFields>(
  state: T,
  selected: { repositoryId: string; repositoryUrl: string; branch: string },
): T {
  return {
    ...state,
    repositoryId: selected.repositoryId,
    repositoryUrl: selected.repositoryUrl,
    branch: selected.branch,
  }
}

export function fieldsAfterConnectionChange<T extends ConnectedSourceFields>(state: T, connectionId: string): T {
  return {
    ...state,
    gitConnectionId: connectionId,
    repositoryId: '',
    repositoryUrl: '',
    branch: DEFAULT_GIT_BRANCH,
  }
}

export function fieldsAfterOrganizationChange<T extends ConnectedSourceFields>(state: T): T {
  return {
    ...state,
    gitConnectionId: '',
    repositoryId: '',
    repositoryUrl: '',
    branch: DEFAULT_GIT_BRANCH,
  }
}

export function fieldsAfterLeavingGit<T extends ConnectedSourceFields>(state: T): T {
  return {
    ...state,
    gitConnectionId: '',
    repositoryId: '',
    repositoryUrl: '',
  }
}

/** Connected repository mode to public Git. The derived clone URL is not reused. */
export function fieldsAfterPublicGit<T extends ConnectedSourceFields>(state: T): T {
  return fieldsAfterLeavingGit(state)
}

export function gitSourceCreateConfig<
  T extends {
    sourceType: string
    repositoryUrl?: string | null
    gitConnectionId?: string | null
    clearGitConnection?: boolean
  },
>(config: T, input: GitSourceCreateInput): GitSourceCreateResult<GitSourceConfig<T>> {
  const rest = omitConnectionKeys(config)
  if (input.repositorySource === 'connected' && config.sourceType === 'git') {
    const selected = selectConnectedRepository({
      organizationId: input.organizationId,
      connection: input.connection,
      repository: input.repository ?? {
        id: '',
        connectionId: '',
        organizationId: '',
        cloneUrl: '',
      },
    })
    if (!selected.ok) return selected
    return {
      ok: true,
      config: {
        ...rest,
        repositoryUrl: selected.repositoryUrl,
        gitConnectionId: input.connection.id,
      },
    }
  }
  return { ok: true, config: rest }
}

type GitSourceConfig<T extends { gitConnectionId?: string | null; clearGitConnection?: boolean }> = Omit<
  T,
  'gitConnectionId' | 'clearGitConnection'
> & { gitConnectionId?: string }

export function validateConnectedGitSubmission(input: {
  organizationId: string
  connection: GitSourceConnection
  repository: GitSourceRepository
}): ConnectedRepositoryResult {
  return selectConnectedRepository(input)
}

export function resetPlacementOnOrganizationChange<
  T extends {
    projectId: string
    serverId: string
    environment?: string
    environmentId?: string
  },
>(state: T): T {
  const next: T = { ...state, projectId: '', serverId: '' }
  if ('environment' in state) next.environment = ''
  if ('environmentId' in state) next.environmentId = ''
  return next
}

export const WIZARD_REPOSITORY_PAGE_LIMIT = REPOSITORY_PAGE_LIMIT

function omitConnectionKeys<
  T extends { gitConnectionId?: string | null; clearGitConnection?: boolean },
>(config: T): Omit<T, 'gitConnectionId' | 'clearGitConnection'> {
  const next = { ...config }
  delete next.gitConnectionId
  delete next.clearGitConnection
  return next
}

function selectionRank(entry: { connection: GitSourceConnection; eligibility: ConnectionEligibility }): number {
  if (!entry.eligibility.selectable) return 2
  if (entry.connection.authMode === 'github_app') return 0
  return 1
}

function isCloneHostname(hostname: string): boolean {
  if (!hostname || hostname === '.' || hostname === '..') return false
  return !hostname.includes('..')
}

function isGitHubProvider(connection: GitSourceConnection): boolean {
  if (typeof connection.provider === 'string' && connection.provider.trim()) {
    return connection.provider.toLowerCase() === 'github'
  }
  return connection.type === 'GitHub'
}

function backendStatus(connection: GitSourceConnection): string {
  if (typeof connection.providerStatus === 'string') return connection.providerStatus
  return connection.status ?? ''
}

function rejectionForConnection(
  connection: GitSourceConnection,
  organizationId: string,
): { code: ConnectedGitRejection; message: string } {
  if (connection.organizationId !== organizationId || !organizationId) {
    return { code: 'organization', message: 'Choose a GitHub connection in this organization.' }
  }
  if (!isGitHubProvider(connection)) {
    return { code: 'provider', message: 'Choose a GitHub connection.' }
  }
  if (connection.authMode !== 'github_app' && connection.authMode !== 'pat') {
    return { code: 'auth_mode', message: 'Choose a GitHub App or personal access token connection.' }
  }
  return { code: 'status', message: 'Choose an active GitHub connection.' }
}
