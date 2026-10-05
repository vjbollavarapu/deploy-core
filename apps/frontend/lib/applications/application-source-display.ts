import {
  credentialFreeCloneUrl,
  gitConnectionEligibility,
  isDeploymentConnectionId,
  repositoryHasCredentialQuery,
  type GitSourceConnection,
} from './git-source'
import { connectionOptionLabel } from './git-source-wizard'

export const SOURCE_UNAVAILABLE = 'Unavailable'
export const SOURCE_LOADING = 'Loading…'

export type ApplicationSourceClass = 'connected-git' | 'public-git' | 'image' | 'compose' | 'other'

export interface ApplicationSourceFacts {
  sourceType: string | null
  repositoryUrl: string | null
  gitBranch: string | null
  dockerfilePath: string | null
  buildContext: string | null
  imageReference: string | null
  gitConnectionId: string | null
}

export interface ConnectedRepositoryCandidate {
  id: string
  connectionId: string
  organizationId: string
  fullName: string
  cloneUrl: string
}

export interface ConnectedSourceMatchInput {
  organizationId: string
  gitConnectionId: string
  repositoryUrl: string | null
  connections: readonly (GitSourceConnection & { account?: string })[]
  repositories: readonly ConnectedRepositoryCandidate[]
}

export interface ConnectedSourceMatch {
  connectionLabel: string | null
  repositoryLabel: string | null
}

export type SourceMetadataStatus = 'idle' | 'loading' | 'resolved' | 'unavailable'

export interface ConnectedGitDisplayMetadata {
  status: SourceMetadataStatus
  connectionLabel: string | null
  repositoryLabel: string | null
}

export const IDLE_GIT_METADATA: ConnectedGitDisplayMetadata = {
  status: 'idle',
  connectionLabel: null,
  repositoryLabel: null,
}

export interface SourceResolutionScope {
  organizationId: string
  applicationId: string
  gitConnectionId: string
  repositoryUrl: string | null
}

export interface SourceDisplayRow {
  label: string
  text: string
  href: string | null
}

export function classifyApplicationSource(facts: ApplicationSourceFacts): ApplicationSourceClass {
  const sourceType = facts.sourceType?.trim().toLowerCase() ?? ''
  const connectionId = facts.gitConnectionId?.trim() ?? ''
  if (sourceType === 'git' && connectionId.length > 0) return 'connected-git'
  if (sourceType === 'git') return 'public-git'
  if (sourceType === 'image') return 'image'
  if (sourceType === 'compose') return 'compose'
  return 'other'
}

/** A late Git response may render only when it still belongs to the visible application and organization. */
export function acceptSourceResolution(requested: SourceResolutionScope, current: SourceResolutionScope): boolean {
  return (
    requested.organizationId === current.organizationId &&
    requested.applicationId === current.applicationId &&
    requested.gitConnectionId === current.gitConnectionId &&
    requested.repositoryUrl === current.repositoryUrl
  )
}

export function connectedSourceMatch(input: ConnectedSourceMatchInput): ConnectedSourceMatch {
  const connection = input.connections.find((item) => item.id === input.gitConnectionId)
  if (!connection || !isDeploymentConnectionId(connection.id)) {
    return { connectionLabel: null, repositoryLabel: null }
  }
  const eligibility = gitConnectionEligibility(connection, input.organizationId)
  if (eligibility.visibility === 'hidden') {
    return { connectionLabel: null, repositoryLabel: null }
  }
  const connectionLabel = connectionOptionLabel({
    account: connection.account,
    selectable: eligibility.selectable,
    authModeLabel: eligibility.authModeLabel,
    statusLabel: eligibility.statusLabel,
  })
  const applicationUrl = credentialFreeCloneUrl(input.repositoryUrl)
  if (!applicationUrl) return { connectionLabel, repositoryLabel: null }
  const repository = input.repositories.find((item) => {
    if (item.connectionId !== connection.id) return false
    if (item.organizationId !== input.organizationId) return false
    return credentialFreeCloneUrl(item.cloneUrl) === applicationUrl
  })
  const name = repository?.fullName.trim() ?? ''
  return { connectionLabel, repositoryLabel: name.length > 0 ? name : null }
}

export function publicRepositoryDisplay(url: string | null | undefined): { text: string; href: string | null } {
  const trimmed = url?.trim() ?? ''
  if (!trimmed) return { text: '—', href: null }
  if (repositoryHasCredentialQuery(trimmed)) return { text: SOURCE_UNAVAILABLE, href: null }
  const safe = credentialFreeCloneUrl(trimmed)
  if (safe) return { text: safe, href: safe }
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed) || trimmed.includes('@')) {
    return { text: SOURCE_UNAVAILABLE, href: null }
  }
  return { text: trimmed, href: null }
}

export function applicationSourceRows(
  facts: ApplicationSourceFacts,
  metadata: ConnectedGitDisplayMetadata,
  variant: 'summary' | 'detail',
): SourceDisplayRow[] {
  const kind = classifyApplicationSource(facts)
  if (kind === 'connected-git') return connectedRows(facts, metadata, variant)
  if (kind === 'public-git') return publicRows(facts, variant)
  if (kind === 'image') return imageRows(facts)
  if (kind === 'compose') return composeRows(facts, variant)
  return [{ label: 'Source', text: present(facts.sourceType), href: null }]
}

function connectedRows(
  facts: ApplicationSourceFacts,
  metadata: ConnectedGitDisplayMetadata,
  variant: 'summary' | 'detail',
): SourceDisplayRow[] {
  const repository = metadataText(metadata, metadata.repositoryLabel)
  const rows: SourceDisplayRow[] = [
    { label: 'Source', text: 'Git', href: null },
    { label: 'Repository source', text: 'Connected repository', href: null },
  ]
  if (variant === 'detail') {
    rows.push({ label: 'Connection', text: metadataText(metadata, metadata.connectionLabel), href: null })
  }
  rows.push(
    { label: 'Repository', text: repository, href: null },
    { label: 'Branch', text: present(facts.gitBranch), href: null },
  )
  if (variant === 'detail') {
    rows.push(
      { label: 'Dockerfile', text: present(facts.dockerfilePath), href: null },
      { label: 'Build context', text: present(facts.buildContext), href: null },
    )
  }
  return rows
}

function publicRows(facts: ApplicationSourceFacts, variant: 'summary' | 'detail'): SourceDisplayRow[] {
  const repository = publicRepositoryDisplay(facts.repositoryUrl)
  const rows: SourceDisplayRow[] = [
    { label: 'Source', text: 'Git', href: null },
    { label: 'Repository source', text: 'Public Git', href: null },
    { label: 'Repository', text: repository.text, href: repository.href },
    { label: 'Branch', text: present(facts.gitBranch), href: null },
  ]
  if (variant === 'detail') {
    rows.push(
      { label: 'Dockerfile', text: present(facts.dockerfilePath), href: null },
      { label: 'Build context', text: present(facts.buildContext), href: null },
    )
  }
  return rows
}

function imageRows(facts: ApplicationSourceFacts): SourceDisplayRow[] {
  return [
    { label: 'Source', text: 'Image', href: null },
    { label: 'Image', text: present(facts.imageReference), href: null },
  ]
}

function composeRows(facts: ApplicationSourceFacts, variant: 'summary' | 'detail'): SourceDisplayRow[] {
  const repository = publicRepositoryDisplay(facts.repositoryUrl)
  const rows: SourceDisplayRow[] = [
    { label: 'Source', text: 'Compose', href: null },
    { label: 'Repository', text: repository.text, href: repository.href },
    { label: 'Branch', text: present(facts.gitBranch), href: null },
  ]
  if (variant === 'detail') {
    rows.push({ label: 'Build context', text: present(facts.buildContext), href: null })
  }
  return rows
}

function metadataText(metadata: ConnectedGitDisplayMetadata, label: string | null): string {
  if (metadata.status === 'loading' || metadata.status === 'idle') return SOURCE_LOADING
  const text = label?.trim() ?? ''
  return text.length > 0 ? text : SOURCE_UNAVAILABLE
}

function present(value: string | null | undefined): string {
  const text = value?.trim() ?? ''
  return text.length > 0 ? text : '—'
}
