import { collectRepositoryPages, repositoryVisibility, type RepositoryPage } from '../github/providers'
import type { RepositorySourceMode } from '../validations/application'
import {
  classifyGitConnections,
  collectGitConnectionPages,
  fieldsAfterConnectionChange,
  fieldsAfterLeavingGit,
  fieldsAfterOrganizationChange,
  fieldsAfterPublicGit,
  fieldsAfterRepositorySelection,
  gitSourceCreateConfig,
  resetPlacementOnOrganizationChange,
  selectConnectedRepository,
  type ConnectedGitRejection,
  type ConnectedSourceFields,
  type GitSourceConnection,
  type GitSourceCreateResult,
  type GitSourceRepository,
} from './git-source'

export const GIT_CONNECTION_LOADING = 'Loading Git connections…'
export const GIT_CONNECTION_ERROR = 'Unable to load Git connections'
export const GIT_CONNECTION_NONE = 'No Git connections available'
export const GIT_CONNECTION_NONE_ACTIVE = 'No active Git connections'
export const GIT_CONNECTION_NONE_HINT = 'Public Git stays available.'
export const GIT_REPOSITORY_NEED_CONNECTION = 'Select a connection first'
export const GIT_REPOSITORY_LOADING = 'Loading repositories…'
export const GIT_REPOSITORY_ERROR = 'Unable to load repositories'
export const GIT_REPOSITORY_NONE = 'No synchronized repositories'
export const GIT_REPOSITORY_NONE_HINT = 'Repositories are managed from Git Providers.'
export const GIT_REVIEW_UNAVAILABLE = 'Unavailable'

export type GitSourceLoadPhase = 'idle' | 'loading' | 'error' | 'ready'

export interface WizardGitFormState {
  sourceType: string
  repositorySource: RepositorySourceMode
  gitConnectionId: string
  repositoryId: string
  repository: string
  branch: string
  dockerfile: string
  buildContext: string
  projectId: string
  environment: string
  serverId: string
}

export interface WizardConnectionInput extends GitSourceConnection {
  account?: string
}

export interface WizardConnectionOption {
  id: string
  account: string
  organizationId: string
  authMode: string
  providerStatus: string
  type: string
  provider?: string
  selectable: boolean
  visibility: 'selectable' | 'unavailable'
  statusLabel: string
  label: string
}

export interface WizardRepositoryInput {
  id: string
  connectionId: string
  organizationId: string
  fullName: string
  defaultBranch?: string | null
  cloneUrl: string
  metadata?: {
    private?: boolean
    archived?: boolean
    visibility?: string
  }
}

export interface WizardRepositoryOption {
  id: string
  label: string
  selectable: boolean
  /** Set only after the clone URL passes the credential-free check. */
  selection?: GitSourceRepository
}

export interface RepositoryLoadScope {
  open: boolean
  organizationId: string
  connectionId: string
  repositorySource: RepositorySourceMode
  sourceType: string
  generation: number
}

export interface ConnectionLoadScope {
  open: boolean
  organizationId: string
  sourceType: string
  generation: number
}

export function loadWizardGitConnections<T>(
  organizationId: string,
  fetchPage: (request: { organizationId: string; limit: number; offset: number }) => Promise<RepositoryPage<T>>,
): Promise<T[]> {
  return collectGitConnectionPages(organizationId, fetchPage)
}

export function loadWizardRepositories<T>(
  fetchPage: (page: { limit: number; offset: number }) => Promise<RepositoryPage<T>>,
): Promise<T[]> {
  return collectRepositoryPages(fetchPage)
}

export function wizardConnectionInputFromMapped(connection: {
  id: string
  organizationId?: string
  type?: string
  provider?: string
  authMode?: string
  providerStatus?: string
  account?: string
}): WizardConnectionInput {
  return {
    id: connection.id,
    organizationId: connection.organizationId,
    type: connection.type,
    provider: connection.provider,
    authMode: connection.authMode,
    providerStatus: connection.providerStatus,
    account: connection.account,
  }
}

export function connectionOptionLabel(input: {
  account?: string
  selectable: boolean
  authModeLabel: string | null
  statusLabel: string
}): string {
  const account = input.account?.trim() || 'GitHub'
  if (input.selectable && input.authModeLabel) {
    return `GitHub · ${account} · ${input.authModeLabel}`
  }
  return `GitHub · ${account} · ${input.statusLabel}`
}

export function wizardConnectionOptions(
  connections: readonly WizardConnectionInput[],
  organizationId: string,
): WizardConnectionOption[] {
  return classifyGitConnections(connections, organizationId)
    .filter((item) => item.eligibility.visibility !== 'hidden')
    .map((item) => {
      const account = item.connection.account?.trim() || 'GitHub'
      const visibility = item.eligibility.visibility === 'selectable' ? 'selectable' : 'unavailable'
      return {
        id: item.connection.id,
        account,
        organizationId: item.connection.organizationId ?? '',
        authMode: item.connection.authMode ?? '',
        providerStatus: item.connection.providerStatus ?? '',
        type: item.connection.type ?? '',
        provider: item.connection.provider,
        selectable: item.eligibility.selectable,
        visibility,
        statusLabel: item.eligibility.statusLabel,
        label: connectionOptionLabel({
          account,
          selectable: item.eligibility.selectable,
          authModeLabel: item.eligibility.authModeLabel,
          statusLabel: item.eligibility.statusLabel,
        }),
      }
    })
}

export function connectionFromOption(option: WizardConnectionOption): GitSourceConnection {
  return {
    id: option.id,
    organizationId: option.organizationId,
    provider: option.provider,
    type: option.type,
    authMode: option.authMode,
    providerStatus: option.providerStatus,
  }
}

export function repositoryOptionLabel(repository: Pick<WizardRepositoryInput, 'fullName' | 'metadata'>): string {
  const name = repository.fullName.trim() || 'Repository'
  const visibility = repositoryVisibility(repository.metadata)
  const parts = [name]
  if (visibility.visibility) parts.push(visibility.visibility)
  if (visibility.archived) parts.push('Archived')
  return parts.join(' · ')
}

export function wizardRepositoryOptions(input: {
  organizationId: string
  connection: GitSourceConnection
  repositories: readonly WizardRepositoryInput[]
}): WizardRepositoryOption[] {
  return input.repositories.map((repository) => {
    const label = repositoryOptionLabel(repository)
    const source: GitSourceRepository = {
      id: repository.id,
      connectionId: repository.connectionId,
      organizationId: repository.organizationId,
      cloneUrl: repository.cloneUrl,
      defaultBranch: repository.defaultBranch,
    }
    const selected = selectConnectedRepository({
      organizationId: input.organizationId,
      connection: input.connection,
      repository: source,
    })
    if (!selected.ok) {
      return { id: repository.id, label, selectable: false }
    }
    return {
      id: repository.id,
      label,
      selectable: true,
      selection: {
        id: selected.repositoryId,
        connectionId: repository.connectionId,
        organizationId: repository.organizationId,
        cloneUrl: selected.repositoryUrl,
        defaultBranch: repository.defaultBranch,
      },
    }
  })
}

export function defaultRepositorySource(input: {
  selectableCount: number
  repository: string
  gitConnectionId: string
  repositoryId: string
  modeChosen: boolean
  current: RepositorySourceMode
}): RepositorySourceMode | null {
  if (input.modeChosen || input.current === 'connected') return null
  if (input.repository.trim() || input.gitConnectionId.trim() || input.repositoryId.trim()) return null
  if (input.selectableCount > 0) return 'connected'
  return null
}

export function connectionListMessage(input: {
  phase: GitSourceLoadPhase
  visibleCount: number
  selectableCount: number
}): string | null {
  if (input.phase === 'loading') return GIT_CONNECTION_LOADING
  if (input.phase === 'error') return GIT_CONNECTION_ERROR
  if (input.phase === 'ready' && input.visibleCount === 0) return GIT_CONNECTION_NONE
  if (input.phase === 'ready' && input.selectableCount === 0) return GIT_CONNECTION_NONE_ACTIVE
  return null
}

export function repositoryListMessage(input: {
  connectionSelected: boolean
  phase: GitSourceLoadPhase
  repositoryCount: number
}): string | null {
  if (!input.connectionSelected) return GIT_REPOSITORY_NEED_CONNECTION
  if (input.phase === 'loading' || input.phase === 'idle') return GIT_REPOSITORY_LOADING
  if (input.phase === 'error') return GIT_REPOSITORY_ERROR
  if (input.repositoryCount === 0) return GIT_REPOSITORY_NONE
  return null
}

export function readGitForm(values: WizardGitFormState): WizardGitFormState {
  return {
    sourceType: values.sourceType,
    repositorySource: values.repositorySource,
    gitConnectionId: values.gitConnectionId,
    repositoryId: values.repositoryId,
    repository: values.repository,
    branch: values.branch,
    dockerfile: values.dockerfile,
    buildContext: values.buildContext,
    projectId: values.projectId,
    environment: values.environment,
    serverId: values.serverId,
  }
}

export function applyRepositorySelection<T extends WizardGitFormState>(
  state: T,
  input: {
    organizationId: string
    connection: GitSourceConnection
    repository: GitSourceRepository
  },
): { ok: true; state: T } | { ok: false; state: T; code: ConnectedGitRejection; message: string } {
  const selected = selectConnectedRepository(input)
  if (!selected.ok) return { ok: false, state, code: selected.code, message: selected.message }
  return {
    ok: true,
    state: withConnectedFields(state, fieldsAfterRepositorySelection(toConnected(state), selected)),
  }
}

export function applyConnectionChange<T extends WizardGitFormState>(state: T, connectionId: string): T {
  return withConnectedFields(state, fieldsAfterConnectionChange(toConnected(state), connectionId))
}

export function applyPublicGitTransition<T extends WizardGitFormState>(state: T): T {
  const cleared = withConnectedFields(state, fieldsAfterPublicGit(toConnected(state)))
  return { ...cleared, repositorySource: 'public' }
}

export function applyConnectedGitTransition<T extends WizardGitFormState>(state: T): T {
  const cleared = withConnectedFields(state, fieldsAfterLeavingGit(toConnected(state)))
  return { ...cleared, repositorySource: 'connected' }
}

export function applySourceTypeChange<T extends WizardGitFormState>(state: T, nextSource: string): T {
  if (state.sourceType === 'git' && nextSource !== 'git') {
    return { ...applyLeavingGit(state), sourceType: nextSource }
  }
  return { ...state, sourceType: nextSource }
}

export function applyLeavingGit<T extends WizardGitFormState>(state: T): T {
  return withConnectedFields(state, fieldsAfterLeavingGit(toConnected(state)))
}

export function applyOrganizationChange<T extends WizardGitFormState>(state: T): T {
  const cleared = withConnectedFields(state, fieldsAfterOrganizationChange(toConnected(state)))
  return resetPlacementOnOrganizationChange(cleared)
}

export function connectionSelectionAfterRefresh<T extends WizardGitFormState>(
  state: T,
  options: readonly WizardConnectionOption[],
  organizationId: string,
): T {
  if (state.repositorySource !== 'connected' || !state.gitConnectionId) return state
  const selected = options.find((option) => option.id === state.gitConnectionId)
  const stillValid =
    selected?.selectable === true &&
    selected.organizationId === organizationId &&
    (selected.authMode === 'github_app' || selected.authMode === 'pat') &&
    selected.providerStatus === 'active'
  if (stillValid) return state
  return applyConnectionChange(state, '')
}

export function repositoryLoadApplies(requested: RepositoryLoadScope, current: RepositoryLoadScope): boolean {
  return (
    requested.open &&
    current.open &&
    requested.generation === current.generation &&
    requested.organizationId === current.organizationId &&
    requested.connectionId === current.connectionId &&
    requested.repositorySource === 'connected' &&
    current.repositorySource === 'connected' &&
    requested.sourceType === 'git' &&
    current.sourceType === 'git'
  )
}

export function repositoryLoadResult<T>(
  requested: RepositoryLoadScope,
  current: RepositoryLoadScope,
  repositories: T[],
): T[] | null {
  if (!repositoryLoadApplies(requested, current)) return null
  return repositories
}

export function connectionLoadApplies(requested: ConnectionLoadScope, current: ConnectionLoadScope): boolean {
  return (
    requested.open &&
    current.open &&
    requested.generation === current.generation &&
    requested.organizationId === current.organizationId &&
    requested.sourceType === 'git' &&
    current.sourceType === 'git'
  )
}

/** Connection and repository lists fetched for one organization. */
export interface WizardGitCache {
  organizationId: string
  connections: WizardConnectionOption[]
  repositories: WizardRepositoryOption[]
  connectionPhase: GitSourceLoadPhase
  repositoryPhase: GitSourceLoadPhase
}

export function emptyWizardGitCache(organizationId = ''): WizardGitCache {
  return {
    organizationId,
    connections: [],
    repositories: [],
    connectionPhase: 'idle',
    repositoryPhase: 'idle',
  }
}

/**
 * Lists the UI is allowed to show. A cache fetched for another organization
 * is empty for the active organization, including before an effect clears it.
 */
export function visibleWizardGitCache(cache: WizardGitCache, activeOrganizationId: string): WizardGitCache {
  if (cache.organizationId === activeOrganizationId) return cache
  return emptyWizardGitCache(activeOrganizationId)
}

/** Drop cached Git data when the active organization changes. Same-org cache is kept. */
export function wizardGitCacheAfterOrganizationChange(cache: WizardGitCache, nextOrganizationId: string): WizardGitCache {
  if (cache.organizationId === nextOrganizationId) return cache
  return emptyWizardGitCache(nextOrganizationId)
}

export function wizardGitCacheAfterConnectionLoading(
  cache: WizardGitCache,
  input: { activeOrganizationId: string; requestedOrganizationId: string },
): WizardGitCache | null {
  if (!ownsGitRequest(input)) return null
  if (cache.organizationId !== input.activeOrganizationId) {
    return { ...emptyWizardGitCache(input.activeOrganizationId), connectionPhase: 'loading' }
  }
  return { ...cache, connectionPhase: 'loading' }
}

export function wizardGitCacheAfterConnectionsLoaded(
  cache: WizardGitCache,
  input: {
    activeOrganizationId: string
    requestedOrganizationId: string
    connections: readonly WizardConnectionOption[]
  },
): WizardGitCache | null {
  if (!ownsGitRequest(input)) return null
  const sameOwner = cache.organizationId === input.activeOrganizationId
  return {
    organizationId: input.activeOrganizationId,
    connections: input.connections.filter((connection) => connection.organizationId === input.activeOrganizationId),
    repositories: sameOwner ? cache.repositories : [],
    connectionPhase: 'ready',
    repositoryPhase: sameOwner ? cache.repositoryPhase : 'idle',
  }
}

/**
 * A failed reload must not keep another organization's list.
 * A same-organization failure keeps that organization's current list.
 */
export function wizardGitCacheAfterConnectionFailure(
  cache: WizardGitCache,
  input: { activeOrganizationId: string; requestedOrganizationId: string },
): WizardGitCache | null {
  if (!ownsGitRequest(input)) return null
  if (cache.organizationId !== input.activeOrganizationId) {
    return { ...emptyWizardGitCache(input.activeOrganizationId), connectionPhase: 'error' }
  }
  return { ...cache, connectionPhase: 'error' }
}

export function wizardGitCacheAfterRepositoriesLoaded(
  cache: WizardGitCache,
  input: {
    activeOrganizationId: string
    requestedOrganizationId: string
    repositories: readonly WizardRepositoryOption[]
    phase: 'loading' | 'ready' | 'error' | 'idle'
  },
): WizardGitCache | null {
  if (!ownsGitRequest(input)) return null
  if (cache.organizationId !== input.activeOrganizationId) {
    return emptyWizardGitCache(input.activeOrganizationId)
  }
  if (input.phase === 'loading') {
    return { ...cache, repositories: [], repositoryPhase: 'loading' }
  }
  if (input.phase === 'error') {
    return { ...cache, repositories: [], repositoryPhase: 'error' }
  }
  if (input.phase === 'idle') {
    if (cache.repositories.length === 0 && cache.repositoryPhase === 'idle') return cache
    return { ...cache, repositories: [], repositoryPhase: 'idle' }
  }
  return { ...cache, repositories: [...input.repositories], repositoryPhase: 'ready' }
}

export function connectionSelectionAllowed(
  option: WizardConnectionOption | undefined,
  activeOrganizationId: string,
): option is WizardConnectionOption {
  return Boolean(
    option &&
      option.selectable &&
      activeOrganizationId.length > 0 &&
      option.organizationId === activeOrganizationId,
  )
}

/** Apply a cache update, or drop data owned by a different organization. */
export function commitWizardGitCache(
  current: WizardGitCache,
  activeOrganizationId: string,
  next: WizardGitCache | null,
): WizardGitCache {
  if (next?.organizationId === activeOrganizationId) return next
  if (current.organizationId !== activeOrganizationId) return wizardGitCacheAfterOrganizationChange(current, activeOrganizationId)
  return current
}

function ownsGitRequest(input: { activeOrganizationId: string; requestedOrganizationId: string }): boolean {
  return input.activeOrganizationId.length > 0 && input.requestedOrganizationId === input.activeOrganizationId
}

function toConnected<T extends WizardGitFormState>(state: T): T & ConnectedSourceFields {
  return { ...state, repositoryUrl: state.repository }
}

function withConnectedFields<T extends WizardGitFormState>(state: T, fields: ConnectedSourceFields): T {
  return {
    ...state,
    gitConnectionId: fields.gitConnectionId,
    repositoryId: fields.repositoryId,
    repository: fields.repositoryUrl,
    branch: fields.branch,
  }
}

export interface WizardCreateSourceInput {
  sourceType: string
  repositorySource: RepositorySourceMode
  organizationId: string
  gitConnectionId: string
  repositoryId: string
  repositoryUrl: string | null
  gitBranch: string | null
  dockerfilePath: string | null
  buildContext: string | null
  connections: readonly WizardConnectionOption[]
  repositories: readonly WizardRepositoryOption[]
}

export interface WizardSourceConfig {
  sourceType: string
  repositoryUrl?: string | null
  gitBranch?: string | null
  dockerfilePath?: string | null
  buildContext?: string | null
  gitConnectionId?: string
}

/** Final create-boundary source config. Connected Git is validated by gitSourceCreateConfig. */
export function wizardCreateSourceConfig(
  input: WizardCreateSourceInput,
): GitSourceCreateResult<WizardSourceConfig> {
  const base: WizardSourceConfig = {
    sourceType: input.sourceType,
    repositoryUrl: input.repositoryUrl,
    gitBranch: input.gitBranch,
    dockerfilePath: input.dockerfilePath,
    buildContext: input.buildContext,
  }
  if (input.repositorySource !== 'connected' || input.sourceType !== 'git') {
    return gitSourceCreateConfig(base, { repositorySource: 'public' })
  }
  const listed = input.connections.find((item) => item.id === input.gitConnectionId)
  const connection = listed
    ? connectionFromOption(listed)
    : { id: input.gitConnectionId, organizationId: '' }
  return gitSourceCreateConfig(base, {
    repositorySource: 'connected',
    organizationId: input.organizationId,
    connection,
    repository: repositoryForSubmit(input.repositories, input.repositoryId),
  })
}

export interface WizardGitReviewInput {
  sourceType: string
  repositorySource: RepositorySourceMode
  organizationId: string
  gitConnectionId: string
  repositoryId: string
  repository: string
  branch: string
  dockerfile: string
  buildContext: string
  connections: readonly WizardConnectionOption[]
  repositories: readonly WizardRepositoryOption[]
}

export interface WizardGitReviewRow {
  label: string
  value: string
}

export function wizardGitReviewRows(input: WizardGitReviewInput): WizardGitReviewRow[] {
  if (input.sourceType !== 'git') return []
  const buildRows = [
    { label: 'Branch', value: input.branch },
    { label: 'Dockerfile', value: input.dockerfile },
    { label: 'Build context', value: input.buildContext },
  ]
  if (input.repositorySource !== 'connected') {
    return [
      { label: 'Repository source', value: 'Public Git URL' },
      { label: 'Repository', value: input.repository },
      ...buildRows,
    ]
  }
  const connection = input.connections.find(
    (item) => item.id === input.gitConnectionId && item.organizationId === input.organizationId && input.organizationId.length > 0,
  )
  const repository = input.repositories.find((item) => item.id === input.repositoryId && item.selection)
  return [
    { label: 'Repository source', value: 'Connected repository' },
    { label: 'Connection', value: connection?.label ?? GIT_REVIEW_UNAVAILABLE },
    { label: 'Repository', value: repository?.label ?? GIT_REVIEW_UNAVAILABLE },
    ...buildRows,
  ]
}

export function wizardDisplayedGitSource(input: WizardGitReviewInput): string {
  if (input.repositorySource === 'connected') {
    const repository =
      wizardGitReviewRows(input).find((row) => row.label === 'Repository')?.value ?? GIT_REVIEW_UNAVAILABLE
    return `${repository}:${input.branch}`
  }
  return `${input.repository}:${input.branch}`
}

function repositoryForSubmit(
  repositories: readonly WizardRepositoryOption[],
  repositoryId: string,
): GitSourceRepository {
  const match = repositories.find((item) => item.id === repositoryId)
  if (match?.selection) return match.selection
  return {
    id: repositoryId,
    connectionId: '',
    organizationId: '',
    cloneUrl: '',
  }
}
