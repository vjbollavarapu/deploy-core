import type { RestartPolicy, UpdateApplicationConfigRequest } from '../api/contract'
import {
  credentialFreeCloneUrl,
  gitSourceCreateConfig,
  type ConnectedGitRejection,
  type GitSourceConnection,
  type GitSourceRepository,
} from './git-source'

/**
 * Builds a complete application config for PATCH /applications/{id}.
 *
 * A config PATCH inserts a new version. Omitted repository, branch, Dockerfile,
 * build context, image, port, command, entrypoint, CPU, memory, restart policy,
 * health check, and runtime config are stored as NULL or a default. Restart
 * policy defaults to unless-stopped, and an omitted health check or runtime
 * config becomes {}. gitConnectionId is kept when omitted and removed only when
 * clearGitConnection is true. autoDeployEnabled is left unset so a source edit
 * does not change it. This function does not create a deployment.
 */
export interface ApplicationSourceSnapshot {
  sourceType: string | null
  repositoryUrl: string | null
  gitBranch: string | null
  dockerfilePath: string | null
  buildContext: string | null
  imageReference: string | null
  gitConnectionId: string | null
  internalPort: number | null
  command: string | null
  entrypoint: string | null
  cpuLimitMillis: number | null
  memoryLimitBytes: number | null
  restartPolicy: string | null
  healthCheck: Record<string, unknown> | null
  runtimeConfig: Record<string, unknown> | null
}

export type ApplicationSourceEdit =
  | {
      kind: 'connected-retain'
      gitBranch?: string | null
      dockerfilePath?: string | null
      buildContext?: string | null
      connection: GitSourceConnection
      repository: GitSourceRepository
    }
  | {
      kind: 'connected-to-public'
      repositoryUrl: string
      gitBranch?: string | null
      dockerfilePath?: string | null
      buildContext?: string | null
    }
  | {
      kind: 'public-to-connected'
      gitBranch?: string | null
      dockerfilePath?: string | null
      buildContext?: string | null
      connection: GitSourceConnection
      repository: GitSourceRepository
    }
  | {
      kind: 'public-git'
      repositoryUrl?: string | null
      gitBranch?: string | null
      dockerfilePath?: string | null
      buildContext?: string | null
    }
  | {
      kind: 'connected-to-image'
      imageReference: string
    }
  | {
      kind: 'image'
      imageReference?: string | null
    }
  | {
      kind: 'connected-to-compose'
      repositoryUrl: string
      gitBranch?: string | null
      buildContext?: string | null
    }
  | {
      kind: 'compose'
      repositoryUrl?: string | null
      gitBranch?: string | null
      buildContext?: string | null
    }

export type ApplicationSourceUpdateFailure = ConnectedGitRejection | 'repository' | 'branch' | 'image' | 'source'

export type ApplicationSourceUpdateResult =
  | { ok: true; config: UpdateApplicationConfigRequest }
  | { ok: false; code: ApplicationSourceUpdateFailure; message: string }

const RESTART_POLICIES = new Set<RestartPolicy>(['always', 'unless-stopped', 'on-failure', 'no'])

export function buildApplicationSourceUpdate(input: {
  current: ApplicationSourceSnapshot
  edit: ApplicationSourceEdit
  organizationId: string
}): ApplicationSourceUpdateResult {
  const preserved = preservedConfig(input.current)
  switch (input.edit.kind) {
    case 'connected-retain':
      return connectedGit(input.current, input.edit, input.organizationId, preserved, true)
    case 'public-to-connected':
      return connectedGit(input.current, input.edit, input.organizationId, preserved, false)
    case 'connected-to-public':
      return publicGit(input.current, input.edit, preserved, true)
    case 'public-git':
      return publicGit(input.current, input.edit, preserved, false)
    case 'connected-to-image':
      return imageSource(input.current, input.edit.imageReference, preserved, true)
    case 'image':
      return imageSource(input.current, input.edit.imageReference, preserved, false)
    case 'connected-to-compose':
      return composeSource(input.current, input.edit, preserved, true)
    case 'compose':
      return composeSource(input.current, input.edit, preserved, false)
  }
}

function connectedGit(
  current: ApplicationSourceSnapshot,
  edit: Extract<ApplicationSourceEdit, { kind: 'connected-retain' | 'public-to-connected' }>,
  organizationId: string,
  preserved: UpdateApplicationConfigRequest,
  retainCurrent: boolean,
): ApplicationSourceUpdateResult {
  const branch = editedText(edit.gitBranch, current.gitBranch)
  if (!branch) return failure('branch', 'Branch is required')
  // PATCH does not check connection organization, status, or repository ownership.
  const selected = gitSourceCreateConfig(
    {
      ...preserved,
      sourceType: 'git',
      repositoryUrl: null,
      gitBranch: branch,
      dockerfilePath: editedText(edit.dockerfilePath, current.dockerfilePath),
      buildContext: editedText(edit.buildContext, current.buildContext),
    },
    {
      repositorySource: 'connected',
      organizationId,
      connection: edit.connection,
      repository: edit.repository,
    },
  )
  if (!selected.ok) return failure(selected.code, selected.message)
  if (retainCurrent && selected.config.gitConnectionId !== current.gitConnectionId) {
    return failure('connection_id', 'Select a GitHub connection.')
  }
  const currentUrl = credentialFreeCloneUrl(current.repositoryUrl)
  if (retainCurrent && currentUrl !== selected.config.repositoryUrl) {
    return currentUrl
      ? failure('repository_connection', 'Choose a repository from the selected connection.')
      : failure('clone_url', 'Repository URL must be a credential-free http(s) URL.')
  }
  const config: UpdateApplicationConfigRequest = {
    ...preserved,
    sourceType: 'git',
    repositoryUrl: selected.config.repositoryUrl ?? null,
    gitBranch: branch,
    dockerfilePath: editedText(edit.dockerfilePath, current.dockerfilePath),
    buildContext: editedText(edit.buildContext, current.buildContext),
    gitConnectionId: selected.config.gitConnectionId,
  }
  return { ok: true, config }
}

function publicGit(
  current: ApplicationSourceSnapshot,
  edit: Extract<ApplicationSourceEdit, { kind: 'connected-to-public' | 'public-git' }>,
  preserved: UpdateApplicationConfigRequest,
  clearingConnection: boolean,
): ApplicationSourceUpdateResult {
  if (clearingConnection !== hasConnection(current)) {
    return failure('source', 'Choose a public Git URL.')
  }
  const supplied = edit.repositoryUrl ?? (clearingConnection || edit.repositoryUrl === null ? '' : (current.repositoryUrl ?? ''))
  const repositoryUrl = explicitRepository(supplied)
  if (!repositoryUrl) return failure('repository', 'Repository is required')
  if (clearingConnection && repositoryUrl === current.repositoryUrl) {
    return failure('repository', 'Repository is required')
  }
  const branch = editedText(edit.gitBranch, current.gitBranch)
  if (!branch) return failure('branch', 'Branch is required')
  return {
    ok: true,
    config: {
      ...preserved,
      sourceType: 'git',
      repositoryUrl,
      gitBranch: branch,
      dockerfilePath: editedText(edit.dockerfilePath, current.dockerfilePath),
      buildContext: editedText(edit.buildContext, current.buildContext),
      ...clearedConnection(clearingConnection),
    },
  }
}

function imageSource(
  current: ApplicationSourceSnapshot,
  imageReference: string | null | undefined,
  preserved: UpdateApplicationConfigRequest,
  clearingConnection: boolean,
): ApplicationSourceUpdateResult {
  if (clearingConnection !== hasConnection(current)) {
    return failure('source', 'Image is required')
  }
  const image = explicitImage(imageReference === undefined ? current.imageReference : imageReference)
  if (!image) return failure('image', 'Image is required')
  return {
    ok: true,
    config: {
      ...preserved,
      sourceType: 'image',
      repositoryUrl: clearingConnection ? null : current.repositoryUrl,
      imageReference: image,
      ...clearedConnection(clearingConnection),
    },
  }
}

function composeSource(
  current: ApplicationSourceSnapshot,
  edit: Extract<ApplicationSourceEdit, { kind: 'connected-to-compose' | 'compose' }>,
  preserved: UpdateApplicationConfigRequest,
  clearingConnection: boolean,
): ApplicationSourceUpdateResult {
  if (clearingConnection !== hasConnection(current)) {
    return failure('source', 'Repository containing the compose file is required')
  }
  const supplied = edit.repositoryUrl === undefined ? current.repositoryUrl : edit.repositoryUrl
  const repositoryUrl = explicitRepository(supplied ?? '')
  if (!repositoryUrl) return failure('repository', 'Repository containing the compose file is required')
  if (clearingConnection && repositoryUrl === current.repositoryUrl) {
    return failure('repository', 'Repository containing the compose file is required')
  }
  return {
    ok: true,
    config: {
      ...preserved,
      sourceType: 'compose',
      repositoryUrl,
      gitBranch: editedText(edit.gitBranch, current.gitBranch),
      buildContext: editedText(edit.buildContext, current.buildContext),
      dockerfilePath: current.dockerfilePath,
      ...clearedConnection(clearingConnection),
    },
  }
}

function preservedConfig(current: ApplicationSourceSnapshot): UpdateApplicationConfigRequest {
  const config: UpdateApplicationConfigRequest = {}
  if (current.gitBranch != null) config.gitBranch = current.gitBranch
  if (current.dockerfilePath != null) config.dockerfilePath = current.dockerfilePath
  if (current.buildContext != null) config.buildContext = current.buildContext
  if (current.imageReference != null) config.imageReference = current.imageReference
  if (current.internalPort != null) config.internalPort = current.internalPort
  if (current.command != null) config.command = current.command
  if (current.entrypoint != null) config.entrypoint = current.entrypoint
  if (current.cpuLimitMillis != null) config.cpuLimitMillis = current.cpuLimitMillis
  if (current.memoryLimitBytes != null) config.memoryLimitBytes = current.memoryLimitBytes
  if (current.restartPolicy && RESTART_POLICIES.has(current.restartPolicy as RestartPolicy)) {
    config.restartPolicy = current.restartPolicy as RestartPolicy
  }
  if (current.healthCheck != null) config.healthCheck = structuredClone(current.healthCheck)
  if (current.runtimeConfig != null) config.runtimeConfig = structuredClone(current.runtimeConfig)
  return config
}

function clearedConnection(clearing: boolean): Pick<UpdateApplicationConfigRequest, 'clearGitConnection'> {
  return clearing ? { clearGitConnection: true } : {}
}

function hasConnection(current: ApplicationSourceSnapshot): boolean {
  return Boolean(current.gitConnectionId && current.gitConnectionId.trim())
}

function editedText(edited: string | null | undefined, current: string | null): string | null {
  const value = edited === undefined ? current : edited
  const trimmed = value?.trim() ?? ''
  return trimmed.length > 0 ? trimmed : null
}

function explicitRepository(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed.length < 3) return null
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed) && credentialFreeCloneUrl(trimmed) == null) return null
  return trimmed
}

function explicitImage(value: string | null | undefined): string | null {
  const trimmed = value?.trim() ?? ''
  return trimmed.length > 0 ? trimmed : null
}

function failure(code: ApplicationSourceUpdateFailure, message: string): ApplicationSourceUpdateResult {
  return { ok: false, code, message }
}
