import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  acceptEditorConnections,
  acceptEditorRepositories,
  initialSourceDraft,
  repositoryIdForCurrentUrl,
  runApplicationSourceSave,
  shouldLeaveSourceEditor,
  sourceDraftAfterMode,
  sourceEditorPatch,
  type SourceEditorDraft,
} from './application-source-editor'
import type { ApplicationSourceSnapshot } from './application-source-update'
import type { GitSourceConnection, GitSourceRepository } from './git-source'

const ORG = 'org-a'
const OTHER_ORG = 'org-b'
const APP_ID = '550e8400-e29b-41d4-a716-446655440010'
const CONNECTION_ID = '550e8400-e29b-41d4-a716-446655440000'
const OTHER_CONNECTION = '550e8400-e29b-41d4-a716-446655440002'
const REPO_URL = 'https://github.com/acme/app.git'
const OTHER_URL = 'https://github.com/acme/other.git'
const TOKEN = 'ghp_editor_secret'

function snapshot(overrides: Partial<ApplicationSourceSnapshot> = {}): ApplicationSourceSnapshot {
  return {
    sourceType: 'git',
    repositoryUrl: REPO_URL,
    gitBranch: 'main',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    imageReference: null,
    gitConnectionId: CONNECTION_ID,
    internalPort: 8080,
    command: 'node server.js',
    entrypoint: '/entrypoint.sh',
    cpuLimitMillis: 250,
    memoryLimitBytes: 134217728,
    restartPolicy: 'on-failure',
    healthCheck: { path: '/ready' },
    runtimeConfig: { desiredReplicas: 2 },
    ...overrides,
  }
}

function connection(overrides: Partial<GitSourceConnection> = {}): GitSourceConnection {
  return {
    id: CONNECTION_ID,
    organizationId: ORG,
    provider: 'github',
    authMode: 'github_app',
    providerStatus: 'active',
    ...overrides,
  }
}

function repository(overrides: Partial<GitSourceRepository> = {}): GitSourceRepository {
  return {
    id: 'repo-1',
    connectionId: CONNECTION_ID,
    organizationId: ORG,
    cloneUrl: REPO_URL,
    ...overrides,
  }
}

function draft(overrides: Partial<SourceEditorDraft> = {}): SourceEditorDraft {
  return {
    mode: 'connected-git',
    repositoryUrl: '',
    gitBranch: 'main',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    imageReference: '',
    gitConnectionId: CONNECTION_ID,
    repositoryId: 'repo-1',
    ...overrides,
  }
}

function configOf(result: ReturnType<typeof sourceEditorPatch>) {
  assert.equal(result.ok, true)
  if (!result.ok) throw new Error('expected config')
  return result.config
}

describe('source editor initialization', () => {
  it('starts from connected Git without copying the clone URL into the public field', () => {
    const next = initialSourceDraft(snapshot())
    assert.equal(next.mode, 'connected-git')
    assert.equal(next.gitConnectionId, CONNECTION_ID)
    assert.equal(next.repositoryUrl, '')
    assert.equal(next.gitBranch, 'main')
    assert.equal(next.dockerfilePath, 'Dockerfile')
    assert.equal(next.buildContext, '.')
    assert.equal(JSON.stringify(next).includes('ghp_'), false)
  })

  it('keeps an empty Dockerfile empty and drops credential-bearing public or Compose URLs', () => {
    assert.equal(initialSourceDraft(snapshot({ dockerfilePath: null, buildContext: 'services' })).dockerfilePath, '')
    assert.equal(initialSourceDraft(snapshot({ dockerfilePath: null, buildContext: 'services' })).buildContext, 'services')
    const unsafe = `github.com/acme/app?token=${TOKEN}`
    const publicGit = initialSourceDraft(snapshot({ gitConnectionId: null, repositoryUrl: unsafe }))
    const compose = initialSourceDraft(snapshot({ sourceType: 'compose', gitConnectionId: null, repositoryUrl: `https://user:${TOKEN}@github.com/acme/compose.git` }))
    assert.equal(publicGit.repositoryUrl, '')
    assert.equal(compose.repositoryUrl, '')
    assert.equal(JSON.stringify(publicGit).includes(TOKEN), false)
    assert.equal(JSON.stringify(compose).includes(TOKEN), false)
  })

  it('starts from public Git, image, and Compose', () => {
    const publicGit = initialSourceDraft(snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }))
    assert.equal(publicGit.mode, 'public-git')
    assert.equal(publicGit.repositoryUrl, 'https://github.com/acme/public.git')
    assert.equal(publicGit.gitConnectionId, '')

    const image = initialSourceDraft(snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: REPO_URL, imageReference: 'nginx:1' }))
    assert.equal(image.mode, 'image')
    assert.equal(image.imageReference, 'nginx:1')
    assert.equal(image.repositoryUrl, '')

    const compose = initialSourceDraft(snapshot({ sourceType: 'compose', gitConnectionId: null, repositoryUrl: 'https://github.com/acme/compose.git', buildContext: 'deploy' }))
    assert.equal(compose.mode, 'compose')
    assert.equal(compose.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal(compose.buildContext, 'deploy')
  })
})

describe('source editor transitions', () => {
  it('keeps the same connected repository and preserves runtime fields', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot(),
      draft: draft({ gitBranch: 'release' }),
      organizationId: ORG,
      connection: connection(),
      repository: repository(),
    }))
    assert.equal(config.gitConnectionId, CONNECTION_ID)
    assert.equal(config.repositoryUrl, REPO_URL)
    assert.equal(config.gitBranch, 'release')
    assert.equal('clearGitConnection' in config, false)
    assert.equal(config.internalPort, 8080)
    assert.equal(config.command, 'node server.js')
    assert.equal(config.entrypoint, '/entrypoint.sh')
    assert.equal(config.cpuLimitMillis, 250)
    assert.equal(config.memoryLimitBytes, 134217728)
    assert.equal(config.dockerfilePath, 'Dockerfile')
    assert.equal(config.buildContext, '.')
    assert.equal(config.restartPolicy, 'on-failure')
    assert.deepEqual(config.healthCheck, { path: '/ready' })
    assert.deepEqual(config.runtimeConfig, { desiredReplicas: 2 })
    assert.equal('autoDeployEnabled' in config, false)
  })

  it('validates a different connected repository without clearing the connection', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot(),
      draft: draft({ repositoryId: 'repo-2' }),
      organizationId: ORG,
      connection: connection(),
      repository: repository({ id: 'repo-2', cloneUrl: OTHER_URL }),
    }))
    assert.equal(config.gitConnectionId, CONNECTION_ID)
    assert.equal(config.repositoryUrl, OTHER_URL)
    assert.equal('clearGitConnection' in config, false)
  })

  it('clears the connection when moving to a new public URL', () => {
    const leaving = sourceDraftAfterMode(snapshot(), initialSourceDraft(snapshot()), 'public-git')
    assert.equal(leaving.repositoryUrl, '')
    const config = configOf(sourceEditorPatch({
      current: snapshot(),
      draft: draft({ mode: 'public-git', repositoryUrl: 'https://github.com/acme/public.git', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
    assert.equal(config.repositoryUrl, 'https://github.com/acme/public.git')
  })

  it('moves public Git to a connected repository', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }),
      draft: draft(),
      organizationId: ORG,
      connection: connection(),
      repository: repository(),
    }))
    assert.equal(config.gitConnectionId, CONNECTION_ID)
    assert.equal(config.repositoryUrl, REPO_URL)
    assert.equal('clearGitConnection' in config, false)
  })

  it('updates public Git without a connection flag', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }),
      draft: draft({ mode: 'public-git', repositoryUrl: 'github.com/acme/public', gitBranch: 'next', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(config.repositoryUrl, 'github.com/acme/public')
    assert.equal(config.gitBranch, 'next')
    assert.equal('gitConnectionId' in config, false)
    assert.equal('clearGitConnection' in config, false)
  })

  it('clears connected Git when moving to an image', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot(),
      draft: draft({ mode: 'image', imageReference: 'ghcr.io/acme/app:9' }),
      organizationId: ORG,
    }))
    assert.equal(config.sourceType, 'image')
    assert.equal(config.imageReference, 'ghcr.io/acme/app:9')
    assert.equal(config.repositoryUrl, null)
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
  })

  it('preserves a stored repository URL when only the image reference changes', () => {
    const config = configOf(sourceEditorPatch({
      current: snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: 'https://github.com/acme/notes.git', imageReference: 'nginx:1' }),
      draft: draft({ mode: 'image', imageReference: 'nginx:2', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(config.sourceType, 'image')
    assert.equal(config.imageReference, 'nginx:2')
    assert.equal(config.repositoryUrl, 'https://github.com/acme/notes.git')
    assert.equal('clearGitConnection' in config, false)
  })

  it('clears connected Git when moving to a new Compose source', () => {
    const leaving = sourceDraftAfterMode(snapshot(), initialSourceDraft(snapshot()), 'compose')
    assert.equal(leaving.repositoryUrl, '')
    const config = configOf(sourceEditorPatch({
      current: snapshot(),
      draft: draft({ mode: 'compose', repositoryUrl: 'https://github.com/acme/compose.git', buildContext: 'deploy', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(config.sourceType, 'compose')
    assert.equal(config.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
  })

  it('moves an image to a connected repository and a Compose source to an image or Compose edit', () => {
    const connected = configOf(sourceEditorPatch({
      current: snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: 'https://github.com/acme/notes.git', imageReference: 'nginx:1' }),
      draft: draft(),
      organizationId: ORG,
      connection: connection(),
      repository: repository(),
    }))
    assert.equal(connected.sourceType, 'git')
    assert.equal(connected.gitConnectionId, CONNECTION_ID)
    assert.equal(connected.repositoryUrl, REPO_URL)

    const image = sourceDraftAfterMode(
      snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: REPO_URL, imageReference: 'nginx:1' }),
      initialSourceDraft(snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: REPO_URL, imageReference: 'nginx:1' })),
      'public-git',
    )
    assert.equal(image.repositoryUrl, '')

    const composeCurrent = snapshot({ sourceType: 'compose', gitConnectionId: null, repositoryUrl: 'https://github.com/acme/compose.git', buildContext: 'deploy' })
    const compose = configOf(sourceEditorPatch({
      current: composeCurrent,
      draft: draft({ mode: 'compose', repositoryUrl: 'https://github.com/acme/compose.git', buildContext: 'services', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(compose.sourceType, 'compose')
    assert.equal(compose.buildContext, 'services')
    assert.equal(compose.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal('clearGitConnection' in compose, false)
    assert.equal(compose.internalPort, 8080)

    const composeImage = configOf(sourceEditorPatch({
      current: composeCurrent,
      draft: draft({ mode: 'image', imageReference: 'nginx:2', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    }))
    assert.equal(composeImage.sourceType, 'image')
    assert.equal(composeImage.imageReference, 'nginx:2')
    assert.equal(composeImage.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal('clearGitConnection' in composeImage, false)
  })
})

describe('source editor validation', () => {
  it('rejects a wrong-organization connection and does not produce a config', () => {
    const result = sourceEditorPatch({
      current: snapshot(),
      draft: draft(),
      organizationId: ORG,
      connection: connection({ organizationId: OTHER_ORG }),
      repository: repository({ organizationId: OTHER_ORG }),
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal('config' in result, false)
  })

  it('rejects an inactive connection for a new selection', () => {
    const result = sourceEditorPatch({
      current: snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }),
      draft: draft(),
      organizationId: ORG,
      connection: connection({ providerStatus: 'disabled' }),
      repository: repository(),
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.code, 'status')
  })

  it('rejects a repository from another connection', () => {
    const result = sourceEditorPatch({
      current: snapshot(),
      draft: draft({ repositoryId: 'repo-2' }),
      organizationId: ORG,
      connection: connection(),
      repository: repository({ id: 'repo-2', connectionId: OTHER_CONNECTION, cloneUrl: OTHER_URL }),
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.code, 'repository_connection')
  })

  it('rejects an unsafe repository URL without echoing it or falling back to public Git', () => {
    const unsafe = `github.com/acme/app?token=${TOKEN}`
    const result = sourceEditorPatch({
      current: snapshot(),
      draft: draft({ mode: 'public-git', repositoryUrl: unsafe, gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
    })
    assert.equal(result.ok, false)
    if (!result.ok) {
      assert.equal(result.message, 'Repository is required')
      assert.equal(JSON.stringify(result).includes(TOKEN), false)
      assert.equal('config' in result, false)
    }
  })
})

describe('source editor save', () => {
  it('patches once from the full-config builder and reloads the same application', async () => {
    const bodies: unknown[] = []
    let reloads = 0
    const result = await runApplicationSourceSave({
      current: snapshot(),
      draft: draft({ gitBranch: 'release' }),
      organizationId: ORG,
      applicationId: APP_ID,
      connection: connection(),
      repository: repository(),
      patch: async (config) => {
        bodies.push(config)
      },
      reload: async () => {
        reloads += 1
        return { id: APP_ID }
      },
    })
    assert.equal(result.ok, true)
    assert.equal(bodies.length, 1)
    assert.equal(reloads, 1)
    const config = bodies[0] as { gitBranch?: string; autoDeployEnabled?: boolean; internalPort?: number }
    assert.equal(config.gitBranch, 'release')
    assert.equal(config.internalPort, 8080)
    assert.equal('autoDeployEnabled' in config, false)
  })

  it('does not patch when validation fails', async () => {
    let patches = 0
    const result = await runApplicationSourceSave({
      current: snapshot(),
      draft: draft({ mode: 'public-git', repositoryUrl: `https://user:${TOKEN}@github.com/acme/app.git`, gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
      applicationId: APP_ID,
      patch: async () => {
        patches += 1
      },
      reload: async () => ({ id: APP_ID }),
    })
    assert.equal(result.ok, false)
    assert.equal(patches, 0)
    if (!result.ok) assert.equal(JSON.stringify(result).includes(TOKEN), false)
  })

  it('keeps a failed PATCH from looking successful', async () => {
    const result = await runApplicationSourceSave({
      current: snapshot({ gitConnectionId: null }),
      draft: draft({ mode: 'public-git', repositoryUrl: 'https://github.com/acme/public.git', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
      applicationId: APP_ID,
      patch: async () => {
        throw new Error('control plane unavailable')
      },
      reload: async () => ({ id: APP_ID }),
    })
    assert.deepEqual(result, { ok: false, phase: 'patch', message: 'control plane unavailable' })
    assert.equal(shouldLeaveSourceEditor(result), false)
  })

  it('does not echo a credential from a failed PATCH', async () => {
    const result = await runApplicationSourceSave({
      current: snapshot({ gitConnectionId: null }),
      draft: draft({ mode: 'public-git', repositoryUrl: 'https://github.com/acme/public.git', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
      applicationId: APP_ID,
      patch: async () => {
        throw new Error(`clone failed https://user:${TOKEN}@github.com/acme/app.git`)
      },
      reload: async () => ({ id: APP_ID }),
    })
    assert.equal(result.ok, false)
    if (!result.ok) {
      assert.equal(result.message, 'The source configuration could not be saved.')
      assert.equal(result.message.includes(TOKEN), false)
    }
  })

  it('does not accept a reload for a different application', async () => {
    const result = await runApplicationSourceSave({
      current: snapshot({ gitConnectionId: null }),
      draft: draft({ mode: 'public-git', repositoryUrl: 'https://github.com/acme/public.git', gitConnectionId: '', repositoryId: '' }),
      organizationId: ORG,
      applicationId: APP_ID,
      patch: async () => undefined,
      reload: async () => ({ id: 'other-app' }),
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.phase, 'refresh')
  })

  it('resets a cancelled edit to the authoritative source', () => {
    const current = snapshot()
    const started = initialSourceDraft(current)
    const edited = sourceDraftAfterMode(current, { ...started, gitBranch: 'wip' }, 'public-git')
    assert.notDeepEqual(edited, started)
    assert.deepEqual(initialSourceDraft(current), started)
    assert.equal(started.repositoryUrl, '')
  })
})

describe('source editor stale responses', () => {
  it('rejects connection and repository results from another scope', () => {
    const current = { organizationId: ORG, applicationId: APP_ID }
    assert.equal(acceptEditorConnections(current, current), true)
    assert.equal(acceptEditorConnections({ organizationId: OTHER_ORG, applicationId: APP_ID }, current), false)
    assert.equal(acceptEditorConnections({ organizationId: ORG, applicationId: 'other-app' }, current), false)
    const repositories = { ...current, connectionId: CONNECTION_ID }
    assert.equal(acceptEditorRepositories(repositories, repositories), true)
    assert.equal(acceptEditorRepositories({ ...repositories, connectionId: OTHER_CONNECTION }, repositories), false)
    assert.equal(repositoryIdForCurrentUrl(REPO_URL, [{ id: 'repo-1', selectable: true, cloneUrl: REPO_URL }]), 'repo-1')
    assert.equal(repositoryIdForCurrentUrl(`https://user:${TOKEN}@github.com/acme/app.git`, [{ id: 'repo-1', selectable: true, cloneUrl: REPO_URL }]), '')
  })
})
