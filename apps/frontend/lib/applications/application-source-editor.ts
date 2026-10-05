import type { UpdateApplicationConfigRequest } from '../api/contract'
import {
  buildApplicationSourceUpdate,
  type ApplicationSourceEdit,
  type ApplicationSourceSnapshot,
  type ApplicationSourceUpdateResult,
} from './application-source-update'
import { credentialFreeCloneUrl, repositoryHasCredentialQuery, type GitSourceConnection, type GitSourceRepository } from './git-source'

export type EditorSourceMode = 'connected-git' | 'public-git' | 'image' | 'compose'

export interface SourceEditorDraft {
  mode: EditorSourceMode
  repositoryUrl: string
  gitBranch: string
  dockerfilePath: string
  buildContext: string
  imageReference: string
  gitConnectionId: string
  repositoryId: string
}

export interface EditorConnectionScope {
  organizationId: string
  applicationId: string
}

export interface EditorRepositoryScope extends EditorConnectionScope {
  connectionId: string
}

export type SourceSaveResult =
  | { ok: true }
  | { ok: false; phase: 'validation' | 'patch' | 'refresh'; message: string }

export function initialSourceDraft(current: ApplicationSourceSnapshot): SourceEditorDraft {
  const source = current.sourceType?.trim().toLowerCase() ?? ''
  const connected = source === 'git' && Boolean(current.gitConnectionId?.trim())
  if (connected) {
    return {
      mode: 'connected-git',
      repositoryUrl: '',
      gitBranch: current.gitBranch ?? '',
      dockerfilePath: current.dockerfilePath ?? '',
      buildContext: current.buildContext ?? '',
      imageReference: current.imageReference ?? '',
      gitConnectionId: current.gitConnectionId ?? '',
      repositoryId: '',
    }
  }
  if (source === 'git') {
    return blankDraft('public-git', current, safeEditableRepository(current.repositoryUrl))
  }
  if (source === 'image') {
    return {
      mode: 'image',
      repositoryUrl: '',
      gitBranch: '',
      dockerfilePath: '',
      buildContext: '',
      imageReference: current.imageReference ?? '',
      gitConnectionId: '',
      repositoryId: '',
    }
  }
  if (source === 'compose') {
    return blankDraft('compose', current, safeEditableRepository(current.repositoryUrl))
  }
  return blankDraft('public-git', current, '')
}

export function sourceDraftAfterMode(
  current: ApplicationSourceSnapshot,
  draft: SourceEditorDraft,
  mode: EditorSourceMode,
): SourceEditorDraft {
  if (mode === draft.mode) return draft
  const source = current.sourceType?.trim().toLowerCase() ?? ''
  const connected = source === 'git' && Boolean(current.gitConnectionId?.trim())
  const next: SourceEditorDraft = { ...draft, mode }
  if (mode === 'public-git') {
    next.gitConnectionId = ''
    next.repositoryId = ''
    next.repositoryUrl = connected || source === 'image' ? '' : source === 'git' || source === 'compose' ? safeEditableRepository(current.repositoryUrl) : ''
    next.gitBranch = draft.gitBranch || current.gitBranch || ''
    next.dockerfilePath = draft.dockerfilePath || current.dockerfilePath || ''
    next.buildContext = draft.buildContext || current.buildContext || ''
  } else if (mode === 'connected-git') {
    next.repositoryUrl = ''
    next.repositoryId = ''
    next.gitConnectionId = connected ? (current.gitConnectionId ?? '') : ''
    next.gitBranch = draft.gitBranch || current.gitBranch || ''
    next.dockerfilePath = draft.dockerfilePath || current.dockerfilePath || ''
    next.buildContext = draft.buildContext || current.buildContext || ''
  } else if (mode === 'image') {
    next.repositoryUrl = ''
    next.gitConnectionId = ''
    next.repositoryId = ''
    next.imageReference = source === 'image' ? (current.imageReference ?? '') : draft.imageReference || current.imageReference || ''
  } else {
    next.gitConnectionId = ''
    next.repositoryId = ''
    next.repositoryUrl = connected || source === 'image' ? '' : source === 'git' || source === 'compose' ? safeEditableRepository(current.repositoryUrl) : ''
    next.gitBranch = draft.gitBranch || current.gitBranch || ''
    next.buildContext = draft.buildContext || current.buildContext || ''
  }
  return next
}

export function repositoryIdForCurrentUrl(
  currentUrl: string | null,
  repositories: readonly { id: string; selectable: boolean; cloneUrl?: string | null }[],
): string {
  const safe = credentialFreeCloneUrl(currentUrl)
  if (!safe) return ''
  const match = repositories.find((repository) => repository.selectable && credentialFreeCloneUrl(repository.cloneUrl) === safe)
  return match?.id ?? ''
}

export function acceptEditorConnections(requested: EditorConnectionScope, current: EditorConnectionScope): boolean {
  return requested.organizationId === current.organizationId && requested.applicationId === current.applicationId && requested.organizationId.length > 0
}

export function acceptEditorRepositories(requested: EditorRepositoryScope, current: EditorRepositoryScope): boolean {
  return (
    acceptEditorConnections(requested, current) &&
    requested.connectionId === current.connectionId &&
    requested.connectionId.length > 0
  )
}

export function sourceEditorPatch(input: {
  current: ApplicationSourceSnapshot
  draft: SourceEditorDraft
  organizationId: string
  connection?: GitSourceConnection | null
  repository?: GitSourceRepository | null
}): ApplicationSourceUpdateResult {
  if (input.draft.mode === 'public-git' || input.draft.mode === 'compose') {
    if (repositoryHasCredentialQuery(input.draft.repositoryUrl)) {
      return {
        ok: false,
        code: 'repository',
        message: input.draft.mode === 'compose' ? 'Repository containing the compose file is required' : 'Repository is required',
      }
    }
  }
  const edit = sourceEditForDraft(input.current, input.draft, input.connection ?? null, input.repository ?? null)
  if (!edit.ok) return edit
  return buildApplicationSourceUpdate({
    current: input.current,
    edit: edit.edit,
    organizationId: input.organizationId,
  })
}

export async function runApplicationSourceSave(input: {
  current: ApplicationSourceSnapshot
  draft: SourceEditorDraft
  organizationId: string
  applicationId: string
  connection?: GitSourceConnection | null
  repository?: GitSourceRepository | null
  patch: (config: UpdateApplicationConfigRequest) => Promise<void>
  reload: () => Promise<{ id: string } | null>
}): Promise<SourceSaveResult> {
  const built = sourceEditorPatch(input)
  if (!built.ok) return { ok: false, phase: 'validation', message: built.message }
  try {
    await input.patch(built.config)
  } catch (error) {
    const message = error instanceof Error ? error.message : ''
    return { ok: false, phase: 'patch', message: safeSourceSaveMessage(message) }
  }
  let refreshed: { id: string } | null
  try {
    refreshed = await input.reload()
  } catch {
    refreshed = null
  }
  if (!refreshed || refreshed.id !== input.applicationId) {
    return { ok: false, phase: 'refresh', message: 'The source was saved, but the latest configuration could not be reloaded.' }
  }
  return { ok: true }
}

export function shouldLeaveSourceEditor(result: SourceSaveResult): boolean {
  return result.ok
}

export function safeSourceSaveMessage(message: string): string {
  if (!message.trim() || /token=|access_token=|password=|credential=|@/.test(message)) {
    return 'The source configuration could not be saved.'
  }
  return message
}

function sourceEditForDraft(
  current: ApplicationSourceSnapshot,
  draft: SourceEditorDraft,
  connection: GitSourceConnection | null,
  repository: GitSourceRepository | null,
): { ok: true; edit: ApplicationSourceEdit } | { ok: false; code: 'connection_id' | 'repository_missing'; message: string } {
  const connectedNow = (current.sourceType?.trim().toLowerCase() ?? '') === 'git' && Boolean(current.gitConnectionId?.trim())
  const branch = draft.gitBranch
  const dockerfilePath = draft.dockerfilePath
  const buildContext = draft.buildContext
  if (draft.mode === 'connected-git') {
    if (!connection || !repository) {
      return { ok: false, code: connection ? 'repository_missing' : 'connection_id', message: connection ? 'Choose a repository.' : 'Select a GitHub connection.' }
    }
    const sameConnection = connection.id === current.gitConnectionId
    const sameRepository = sameConnection && credentialFreeCloneUrl(current.repositoryUrl) === credentialFreeCloneUrl(repository.cloneUrl)
    if (connectedNow && sameRepository) {
      return { ok: true, edit: { kind: 'connected-retain', gitBranch: branch, dockerfilePath, buildContext, connection, repository } }
    }
    return { ok: true, edit: { kind: 'public-to-connected', gitBranch: branch, dockerfilePath, buildContext, connection, repository } }
  }
  if (draft.mode === 'public-git') {
    const fields = { repositoryUrl: draft.repositoryUrl, gitBranch: branch, dockerfilePath, buildContext }
    return { ok: true, edit: connectedNow ? { kind: 'connected-to-public', ...fields } : { kind: 'public-git', ...fields } }
  }
  if (draft.mode === 'image') {
    return { ok: true, edit: connectedNow ? { kind: 'connected-to-image', imageReference: draft.imageReference } : { kind: 'image', imageReference: draft.imageReference } }
  }
  const compose = { repositoryUrl: draft.repositoryUrl, gitBranch: branch, buildContext }
  return { ok: true, edit: connectedNow ? { kind: 'connected-to-compose', ...compose } : { kind: 'compose', ...compose } }
}

function safeEditableRepository(value: string | null): string {
  const trimmed = value?.trim() ?? ''
  if (!trimmed || repositoryHasCredentialQuery(trimmed)) return ''
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed) && credentialFreeCloneUrl(trimmed) == null) return ''
  return trimmed
}

function blankDraft(mode: 'public-git' | 'compose', current: ApplicationSourceSnapshot, repositoryUrl: string): SourceEditorDraft {
  return {
    mode,
    repositoryUrl,
    gitBranch: current.gitBranch ?? '',
    dockerfilePath: mode === 'public-git' ? (current.dockerfilePath ?? '') : '',
    buildContext: current.buildContext ?? '',
    imageReference: current.imageReference ?? '',
    gitConnectionId: '',
    repositoryId: '',
  }
}
