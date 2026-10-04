import type {
  GitAccountType,
  GitAuthMode,
  GitProviderConnection,
  GitProviderType,
  GitRepositorySelection,
  Status,
} from '../types'

export type { GitAccountType, GitAuthMode, GitRepositorySelection }

/** Safe Git connection fields returned by the control plane. Credential material is omitted. */
export interface WireGitConnection {
  id: string
  organizationId: string
  provider: string
  accountLogin: string
  displayName: string
  status: string
  lastSyncAt?: string | null
  authMode?: GitAuthMode | string
  installationId?: number | null
  accountId?: string
  accountType?: string
  repositorySelection?: string
  hasWebhookSecret: boolean
  metadata?: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

export interface WireGitRepositoryMetadata {
  private?: boolean
  archived?: boolean
  visibility?: string
}

export interface WireGitRepository {
  id: string
  organizationId: string
  connectionId: string
  externalId: string
  fullName: string
  defaultBranch: string
  cloneUrl: string
  htmlUrl: string
  metadata?: WireGitRepositoryMetadata
  lastSyncAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface GitHubAppStatus {
  configured: boolean
  appId?: string
  slug?: string
}

export interface BeginGitHubInstallationResponse {
  installationUrl: string
  expiresAt: string
}

export interface CompleteGitHubInstallationRequest {
  installationId: number
  setupAction: 'install' | 'update'
  state: string
}

export interface CompleteGitHubInstallationResponse {
  connection: WireGitConnection
  repositoryCount: number
}

export interface SyncGitConnectionResponse {
  connection: WireGitConnection
  repositories: WireGitRepository[]
}

export function mapWireGitConnection(wire: WireGitConnection, repoCount = 0): GitProviderConnection {
  const authMode = normalizeAuthMode(wire.authMode)
  const githubApp = authMode === 'github_app'
  const explicitPermissions = stringList(wire.metadata?.permissions)

  return {
    id: wire.id,
    type: providerType(wire.provider),
    account: wire.accountLogin || wire.displayName || 'Connected Account',
    organizations: stringList(wire.metadata?.organizations).length > 0
      ? stringList(wire.metadata?.organizations)
      : [wire.accountLogin || 'default'],
    repositoryCount: githubApp
      ? nonNegativeCount(repoCount)
      : nonNegativeCount(repoCount) || numericMetadata(wire.metadata?.repositoryCount),
    status: connectionStatus(wire.status),
    permissions: githubApp ? [] : explicitPermissions.length > 0 ? explicitPermissions : ['read:repo', 'read:org'],
    lastSync: wire.lastSyncAt ? new Date(wire.lastSyncAt).toLocaleString() : 'Never',
    organizationId: wire.organizationId,
    displayName: wire.displayName,
    authMode,
    installationId: positiveInstallationId(wire.installationId),
    accountId: wire.accountId ?? '',
    accountType: normalizeAccountType(wire.accountType),
    repositorySelection: normalizeRepositorySelection(wire.repositorySelection),
    hasWebhookSecret: wire.hasWebhookSecret,
    providerStatus: wire.status,
  }
}

function normalizeAuthMode(value: string | undefined): GitAuthMode | string {
  if (value === 'github_app' || value === 'pat') return value
  if (typeof value === 'string' && value.trim()) return value.trim()
  return 'pat'
}

function normalizeAccountType(value: string | undefined): GitAccountType {
  if (value === 'User' || value === 'Organization') return value
  return ''
}

function normalizeRepositorySelection(value: string | undefined): GitRepositorySelection {
  if (value === 'all' || value === 'selected') return value
  return ''
}

function positiveInstallationId(value: number | null | undefined): number | null {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value <= 0) return null
  return value
}

function providerType(provider: string): GitProviderType {
  switch (provider.toLowerCase()) {
    case 'github':
      return 'GitHub'
    case 'gitlab':
      return 'GitLab'
    case 'bitbucket':
      return 'Bitbucket'
    default:
      return 'Generic Git'
  }
}

function connectionStatus(status: string): Status {
  if (status === 'active') return 'healthy'
  if (status === 'error') return 'failed'
  if (status === 'revoked') return 'stopped'
  return 'stopped'
}

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
}

function numericMetadata(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}

function nonNegativeCount(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0
}
