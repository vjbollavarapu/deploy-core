import { hasCapability, type DefaultRoleName } from '../rbac'
import type { Status, StatusTone } from '../types'
import { STATUS_CONFIG } from '../status'
import {
  mapWireGitConnection,
  type SyncGitConnectionResponse,
  type WireGitRepositoryMetadata,
} from './git-connection'
import type { GitProviderConnection } from '../types'

export const REPOSITORY_PAGE_LIMIT = 100
export const REPOSITORY_COUNT_LIMIT = 1

const ROLE_NAMES: Record<string, DefaultRoleName> = {
  owner: 'Owner',
  administrator: 'Administrator',
  devops: 'DevOps',
  developer: 'Developer',
  support: 'Support',
  viewer: 'Viewer',
}

export interface GitConnectionAccess {
  /** False when the current role could not be resolved. Manage actions stay available and the API decides. */
  known: boolean
  canRead: boolean
  canManage: boolean
}

export const GIT_ACCESS_UNKNOWN: GitConnectionAccess = {
  known: false,
  canRead: true,
  canManage: true,
}

export type GitHubAppStatusKind = 'loading' | 'configured' | 'unconfigured' | 'unavailable'

export function gitHubAppStatusKind(input: {
  loading: boolean
  error: string | null
  configured: boolean | null
}): GitHubAppStatusKind {
  if (input.configured === true) return 'configured'
  if (input.configured === false) return 'unconfigured'
  if (input.error) return 'unavailable'
  return 'loading'
}

export function beginGitHubInstallationRequest(organizationId: string): {
  path: '/integrations/github/installations'
  body: { organizationId: string }
} {
  return {
    path: '/integrations/github/installations',
    body: { organizationId },
  }
}

/** Accept only an http(s) URL and return the original string unchanged. */
export function installationNavigationTarget(installationUrl: string | null | undefined): string | null {
  if (!installationUrl) return null
  try {
    const parsed = new URL(installationUrl)
    if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return null
  } catch {
    return null
  }
  return installationUrl
}

export function shouldBeginInstallation(input: {
  pending: boolean
  organizationId: string
  configured: boolean
  canManage: boolean
}): boolean {
  return Boolean(input.organizationId) && input.configured && input.canManage && !input.pending
}

export function gitConnectionSyncRequest(connectionId: string): {
  path: string
  body: Record<string, never>
} {
  return {
    path: `/integrations/git/connections/${connectionId}/sync`,
    body: {},
  }
}

export function gitAccessFromRoles(
  roles: readonly { key?: string; name?: string }[] | null,
): GitConnectionAccess {
  if (roles == null) return GIT_ACCESS_UNKNOWN
  let recognized = false
  let canRead = false
  let canManage = false
  for (const role of roles) {
    const name = roleName(role)
    if (!name) continue
    recognized = true
    if (hasCapability(name, 'git.connection.read')) canRead = true
    if (hasCapability(name, 'git.connection.manage')) canManage = true
  }
  if (!recognized) return GIT_ACCESS_UNKNOWN
  return { known: true, canRead, canManage }
}

export function authModeLabel(authMode: string | undefined): string | null {
  if (authMode === 'github_app') return 'GitHub App'
  if (authMode === 'pat') return 'Personal access token'
  return null
}

export function accountTypeLabel(accountType: string | undefined): string | null {
  if (accountType === 'User') return 'User'
  if (accountType === 'Organization') return 'Organization'
  return null
}

export function repositorySelectionLabel(selection: string | undefined): string | null {
  if (selection === 'all') return 'All repositories'
  if (selection === 'selected') return 'Selected repositories'
  return null
}

export function connectionStatusView(
  providerStatus: string | undefined,
  fallback: Status = 'unknown',
): { label: string; tone: StatusTone } {
  switch (providerStatus) {
    case 'active':
      return { label: 'Active', tone: 'success' }
    case 'error':
      return { label: 'Error', tone: 'critical' }
    case 'disabled':
      return { label: 'Suspended', tone: 'warning' }
    case 'revoked':
      return { label: 'Revoked', tone: 'inactive' }
    default: {
      if (!providerStatus) {
        const config = STATUS_CONFIG[fallback] ?? STATUS_CONFIG.unknown
        return { label: config.label, tone: config.tone }
      }
      return { label: 'Unknown', tone: 'inactive' }
    }
  }
}

export function githubManageUrl(input: {
  authMode?: string
  installationId?: number | null
  accountType?: string
  accountLogin?: string
}): string | null {
  if (input.authMode !== 'github_app') return null
  const id = input.installationId
  if (typeof id !== 'number' || !Number.isSafeInteger(id) || id <= 0) return null
  if (input.accountType === 'User') {
    return `https://github.com/settings/installations/${id}`
  }
  if (input.accountType === 'Organization') {
    const login = input.accountLogin?.trim() ?? ''
    if (!/^[\w.-]+$/.test(login)) return null
    return `https://github.com/organizations/${encodeURIComponent(login)}/settings/installations/${id}`
  }
  return null
}

export function disconnectCopy(connection: {
  authMode?: string
  account: string
  type: string
}): { title: string; description: string } {
  if (connection.authMode === 'github_app') {
    return {
      title: `Disconnect ${connection.account}?`,
      description:
        `DeployCore will disconnect the GitHub installation connection for ${connection.account}. ` +
        'Applications that reference it keep their source configuration. ' +
        'Future private-source deployments that use this connection will fail until a valid connection is attached again. ' +
        'This does not uninstall the GitHub App from GitHub.',
    }
  }
  return {
    title: `Disconnect ${connection.type}?`,
    description:
      `This removes the saved connection for ${connection.account} and stops repository sync from DeployCore. ` +
      'Existing applications keep their source configuration. ' +
      'Future private-source deployments that use this connection will fail until a valid connection is attached again.',
  }
}

export function repositoryTotalCount(page: { totalCount?: number | null }): number | null {
  const value = page.totalCount
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return null
  return value
}

export type RepositoryCountValue = number | 'loading' | 'unavailable'

export function countsForOrganization<T>(
  activeOrganizationId: string,
  snapshot: { organizationId: string; counts: T } | null,
): T | null {
  if (!snapshot || snapshot.organizationId !== activeOrganizationId) return null
  return snapshot.counts
}

export interface RepositoryPage<T> {
  items?: T[]
  totalCount?: number | null
}

export async function collectRepositoryPages<T>(
  fetchPage: (page: { limit: number; offset: number }) => Promise<RepositoryPage<T>>,
): Promise<T[]> {
  const all: T[] = []
  let offset = 0
  for (let pageIndex = 0; pageIndex < 100; pageIndex += 1) {
    const page = await fetchPage({ limit: REPOSITORY_PAGE_LIMIT, offset })
    const items = page.items ?? []
    all.push(...items)
    offset += items.length
    if (items.length < REPOSITORY_PAGE_LIMIT) return all
    if (typeof page.totalCount === 'number' && offset >= page.totalCount) return all
  }
  return all
}

export function applySyncResult(
  response: Pick<SyncGitConnectionResponse, 'connection' | 'repositories'>,
): GitProviderConnection {
  return mapWireGitConnection(response.connection, response.repositories.length)
}

/**
 * Dialog list after a successful sync. The sync payload is not the displayed list:
 * PAT sync with {} can return an empty slice while stored repositories remain.
 */
export async function repositoriesToDisplayAfterSync<T>(input: {
  previous: readonly T[]
  syncRepositories: readonly unknown[]
  loadPages: (page: { limit: number; offset: number }) => Promise<RepositoryPage<T>>
}): Promise<{ repositories: readonly T[]; preservedPrevious: boolean; reloadError: unknown | null }> {
  void input.syncRepositories
  try {
    const repositories = await collectRepositoryPages(input.loadPages)
    return { repositories, preservedPrevious: false, reloadError: null }
  } catch (error) {
    return { repositories: input.previous, preservedPrevious: true, reloadError: error }
  }
}

export function suspendedSyncStatus(error: {
  status?: number
  message?: string
  details?: Record<string, unknown>
}): 'disabled' | null {
  const detail = typeof error.details?.status === 'string' ? error.details.status : ''
  const message = (error.message ?? '').toLowerCase()
  if (error.status === 409 && (detail === 'disabled' || message.includes('suspended'))) return 'disabled'
  return null
}

export function repositoryVisibility(metadata: WireGitRepositoryMetadata | undefined): {
  visibility: 'Private' | 'Public' | null
  archived: boolean
} {
  const archived = metadata?.archived === true
  if (metadata?.private === true || metadata?.visibility === 'private') {
    return { visibility: 'Private', archived }
  }
  if (metadata?.private === false || metadata?.visibility === 'public') {
    return { visibility: 'Public', archived }
  }
  return { visibility: null, archived }
}

/** Drop userinfo so a clone URL cannot become an authenticated link. */
export function safeRepositoryHref(url: string | undefined): string | null {
  if (!url) return null
  try {
    const parsed = new URL(url)
    if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return null
    if (parsed.username || parsed.password) return null
    return url
  } catch {
    return null
  }
}

export function isGitHubAppConnection(connection: { authMode?: string }): boolean {
  return connection.authMode === 'github_app'
}

export function patchConnectionStatus(
  connection: GitProviderConnection,
  providerStatus: 'disabled',
): GitProviderConnection {
  return {
    ...connection,
    providerStatus,
    status: 'degraded',
  }
}

export function roleName(role: { key?: string; name?: string }): DefaultRoleName | null {
  const key = role.key?.trim().toLowerCase()
  if (key && ROLE_NAMES[key]) return ROLE_NAMES[key]
  const name = role.name?.trim().toLowerCase()
  if (name && ROLE_NAMES[name]) return ROLE_NAMES[name]
  return null
}

export async function accessForOrganizationUser(
  userId: string,
  fetchPage: (offset: number) => Promise<{
    items?: Array<{ userId?: string; roles?: Array<{ key?: string; name?: string }> }>
    totalCount?: number | null
  }>,
): Promise<GitConnectionAccess> {
  let offset = 0
  for (let pageIndex = 0; pageIndex < 20; pageIndex += 1) {
    const page = await fetchPage(offset)
    const items = page.items ?? []
    const match = items.find((item) => item.userId === userId)
    if (match) return gitAccessFromRoles(match.roles ?? [])
    offset += items.length
    if (items.length === 0) return GIT_ACCESS_UNKNOWN
    if (typeof page.totalCount === 'number' && offset >= page.totalCount) return GIT_ACCESS_UNKNOWN
    if (items.length < 100) return GIT_ACCESS_UNKNOWN
  }
  return GIT_ACCESS_UNKNOWN
}
