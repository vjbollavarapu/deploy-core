import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  buildApplicationSourceUpdate,
  type ApplicationSourceSnapshot,
} from './application-source-update'
import type { GitSourceConnection, GitSourceRepository } from './git-source'

const ORG = 'org-a'
const OTHER_ORG = 'org-b'
const APP_ID = '550e8400-e29b-41d4-a716-446655440000'
const PAT_ID = '550e8400-e29b-41d4-a716-446655440001'
const REPO_URL = 'https://github.com/acme/app.git'
const TOKEN = 'ghp_secret_token'

function connection(overrides: Partial<GitSourceConnection> = {}): GitSourceConnection {
  return {
    id: APP_ID,
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
    connectionId: APP_ID,
    organizationId: ORG,
    cloneUrl: REPO_URL,
    defaultBranch: 'main',
    ...overrides,
  }
}

function snapshot(overrides: Partial<ApplicationSourceSnapshot> = {}): ApplicationSourceSnapshot {
  return {
    sourceType: 'git',
    repositoryUrl: REPO_URL,
    gitBranch: 'main',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    imageReference: 'ghcr.io/acme/app:1',
    gitConnectionId: APP_ID,
    internalPort: 8080,
    command: 'node server.js',
    entrypoint: '/entrypoint.sh',
    cpuLimitMillis: 250,
    memoryLimitBytes: 134217728,
    restartPolicy: 'on-failure',
    healthCheck: { path: '/ready', port: 8080 },
    runtimeConfig: { desiredReplicas: 3, rolling: true },
    ...overrides,
  }
}

function okConfig(result: ReturnType<typeof buildApplicationSourceUpdate>) {
  assert.equal(result.ok, true)
  if (!result.ok) throw new Error('expected config')
  return result.config
}

describe('connected Git retain', () => {
  it('keeps the connection and credential-free repository URL when the branch changes', () => {
    const current = snapshot()
    const config = okConfig(
      buildApplicationSourceUpdate({
        current,
        organizationId: ORG,
        edit: { kind: 'connected-retain', gitBranch: 'release', connection: connection(), repository: repository() },
      }),
    )
    assert.equal(config.sourceType, 'git')
    assert.equal(config.gitConnectionId, APP_ID)
    assert.equal(config.repositoryUrl, REPO_URL)
    assert.equal(config.gitBranch, 'release')
    assert.equal(config.clearGitConnection, undefined)
    assert.equal('clearGitConnection' in config, false)
  })

  it('keeps the connection when Dockerfile and build context change', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: {
          kind: 'connected-retain',
          dockerfilePath: 'deploy/Dockerfile',
          buildContext: 'services/api',
          connection: connection(),
          repository: repository(),
        },
      }),
    )
    assert.equal(config.gitConnectionId, APP_ID)
    assert.equal(config.dockerfilePath, 'deploy/Dockerfile')
    assert.equal(config.buildContext, 'services/api')
    assert.equal(config.repositoryUrl, REPO_URL)
  })

  it('preserves unrelated runtime fields and does not enable auto deploy', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: { kind: 'connected-retain', gitBranch: 'release', connection: connection(), repository: repository() },
      }),
    )
    assert.equal(config.internalPort, 8080)
    assert.equal(config.command, 'node server.js')
    assert.equal(config.entrypoint, '/entrypoint.sh')
    assert.equal(config.cpuLimitMillis, 250)
    assert.equal(config.memoryLimitBytes, 134217728)
    assert.equal(config.restartPolicy, 'on-failure')
    assert.deepEqual(config.healthCheck, { path: '/ready', port: 8080 })
    assert.deepEqual(config.runtimeConfig, { desiredReplicas: 3, rolling: true })
    assert.equal('desiredReplicas' in config, false)
    assert.equal('autoDeployEnabled' in config, false)
    assert.notEqual(config.autoDeployEnabled, true)
  })
})

describe('source transitions', () => {
  it('clears a connection when moving to a new public repository', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: {
          kind: 'connected-to-public',
          repositoryUrl: 'https://github.com/acme/public.git',
          gitBranch: 'develop',
          dockerfilePath: 'Dockerfile',
          buildContext: '.',
        },
      }),
    )
    assert.equal(config.sourceType, 'git')
    assert.equal(config.repositoryUrl, 'https://github.com/acme/public.git')
    assert.equal(config.gitBranch, 'develop')
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
    assert.notEqual(config.repositoryUrl, REPO_URL)
    assert.equal(config.internalPort, 8080)
  })

  it('does not reuse the connected repository when the public URL is omitted', () => {
    const result = buildApplicationSourceUpdate({
      current: snapshot(),
      organizationId: ORG,
      edit: { kind: 'connected-to-public', repositoryUrl: '', gitBranch: 'main' },
    })
    assert.equal(result.ok, false)
    if (!result.ok) {
      assert.equal(result.message, 'Repository is required')
      assert.equal('config' in result, false)
    }
  })

  it('sends a safe repository URL when moving from public Git to a connection', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }),
        organizationId: ORG,
        edit: { kind: 'public-to-connected', gitBranch: 'main', connection: connection(), repository: repository() },
      }),
    )
    assert.equal(config.gitConnectionId, APP_ID)
    assert.equal(config.repositoryUrl, REPO_URL)
    assert.equal(config.sourceType, 'git')
    assert.equal('clearGitConnection' in config, false)
  })

  it('clears the connection when moving to an image and does not keep the repository URL', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: { kind: 'connected-to-image', imageReference: 'ghcr.io/acme/app:9' },
      }),
    )
    assert.equal(config.sourceType, 'image')
    assert.equal(config.imageReference, 'ghcr.io/acme/app:9')
    assert.equal(config.repositoryUrl, null)
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
    assert.equal(config.command, 'node server.js')
    assert.deepEqual(config.runtimeConfig, { desiredReplicas: 3, rolling: true })
  })

  it('clears the connection when moving to a new Compose source', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: {
          kind: 'connected-to-compose',
          repositoryUrl: 'https://github.com/acme/compose.git',
          gitBranch: 'main',
          buildContext: 'deploy',
        },
      }),
    )
    assert.equal(config.sourceType, 'compose')
    assert.equal(config.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal(config.buildContext, 'deploy')
    assert.equal(config.clearGitConnection, true)
    assert.equal('gitConnectionId' in config, false)
    assert.notEqual(config.repositoryUrl, REPO_URL)
  })

  it('updates public Git, image, and Compose without clearGitConnection when none is stored', () => {
    const publicConfig = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot({ gitConnectionId: null, repositoryUrl: 'https://github.com/acme/public.git' }),
        organizationId: ORG,
        edit: { kind: 'public-git', gitBranch: 'next' },
      }),
    )
    assert.equal(publicConfig.repositoryUrl, 'https://github.com/acme/public.git')
    assert.equal(publicConfig.gitBranch, 'next')
    assert.equal('clearGitConnection' in publicConfig, false)
    assert.equal('gitConnectionId' in publicConfig, false)

    const imageConfig = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot({ sourceType: 'image', gitConnectionId: null, repositoryUrl: null }),
        organizationId: ORG,
        edit: { kind: 'image', imageReference: 'ghcr.io/acme/app:4' },
      }),
    )
    assert.equal(imageConfig.sourceType, 'image')
    assert.equal(imageConfig.imageReference, 'ghcr.io/acme/app:4')
    assert.equal(imageConfig.repositoryUrl, null)
    assert.equal(imageConfig.clearGitConnection, undefined)
    assert.equal('gitConnectionId' in imageConfig, false)

    const composeConfig = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot({
          sourceType: 'compose',
          gitConnectionId: null,
          repositoryUrl: 'https://github.com/acme/compose.git',
        }),
        organizationId: ORG,
        edit: { kind: 'compose', buildContext: 'stack' },
      }),
    )
    assert.equal(composeConfig.sourceType, 'compose')
    assert.equal(composeConfig.repositoryUrl, 'https://github.com/acme/compose.git')
    assert.equal(composeConfig.buildContext, 'stack')
    assert.equal('clearGitConnection' in composeConfig, false)
  })

  it('preserves a stored repository URL when only the image reference changes', () => {
    const config = okConfig(
      buildApplicationSourceUpdate({
        current: snapshot({
          sourceType: 'image',
          gitConnectionId: null,
          repositoryUrl: 'https://github.com/acme/notes.git',
          imageReference: 'ghcr.io/acme/app:1',
        }),
        organizationId: ORG,
        edit: { kind: 'image', imageReference: 'ghcr.io/acme/app:4' },
      }),
    )
    assert.equal(config.sourceType, 'image')
    assert.equal(config.imageReference, 'ghcr.io/acme/app:4')
    assert.equal(config.repositoryUrl, 'https://github.com/acme/notes.git')
    assert.equal('clearGitConnection' in config, false)
    assert.equal('gitConnectionId' in config, false)
    assert.equal(config.internalPort, 8080)
    assert.equal(config.command, 'node server.js')
    assert.equal(config.entrypoint, '/entrypoint.sh')
    assert.equal(config.cpuLimitMillis, 250)
    assert.equal(config.memoryLimitBytes, 134217728)
    assert.equal(config.restartPolicy, 'on-failure')
    assert.deepEqual(config.healthCheck, { path: '/ready', port: 8080 })
    assert.deepEqual(config.runtimeConfig, { desiredReplicas: 3, rolling: true })
    assert.equal('autoDeployEnabled' in config, false)
  })
})

describe('connected edits fail closed', () => {
  const cases: Array<{ name: string; connection: GitSourceConnection; repository: GitSourceRepository; code: string }> = [
    {
      name: 'malformed connection id',
      connection: connection({ id: 'not-a-uuid' }),
      repository: repository({ connectionId: 'not-a-uuid' }),
      code: 'connection_id',
    },
    {
      name: 'wrong organization',
      connection: connection({ organizationId: OTHER_ORG }),
      repository: repository({ organizationId: OTHER_ORG }),
      code: 'organization',
    },
    {
      name: 'inactive connection',
      connection: connection({ providerStatus: 'disabled' }),
      repository: repository(),
      code: 'status',
    },
    {
      name: 'unsupported provider',
      connection: connection({ provider: 'gitlab' }),
      repository: repository(),
      code: 'provider',
    },
    {
      name: 'unsupported auth mode',
      connection: connection({ id: PAT_ID, authMode: 'oauth' }),
      repository: repository({ connectionId: PAT_ID }),
      code: 'auth_mode',
    },
    {
      name: 'repository from another connection',
      connection: connection(),
      repository: repository({ connectionId: PAT_ID }),
      code: 'repository_connection',
    },
    {
      name: 'repository from another organization',
      connection: connection(),
      repository: repository({ organizationId: OTHER_ORG }),
      code: 'repository_organization',
    },
    {
      name: 'unsafe clone URL',
      connection: connection(),
      repository: repository({ cloneUrl: `https://x-access-token:${TOKEN}@github.com/acme/app.git` }),
      code: 'clone_url',
    },
  ]

  for (const entry of cases) {
    it(`rejects a ${entry.name}`, () => {
      const result = buildApplicationSourceUpdate({
        current: snapshot(),
        organizationId: ORG,
        edit: {
          kind: 'connected-retain',
          gitBranch: 'release',
          connection: entry.connection,
          repository: entry.repository,
        },
      })
      assert.equal(result.ok, false)
      if (result.ok) return
      assert.equal(result.code, entry.code)
      assert.equal('config' in result, false)
      assert.equal(result.message.includes(TOKEN), false)
      assert.equal(result.message.includes('x-access-token'), false)
      assert.equal(JSON.stringify(result).includes(TOKEN), false)
    })
  }

  it('rejects a missing repository', () => {
    const result = buildApplicationSourceUpdate({
      current: snapshot(),
      organizationId: ORG,
      edit: {
        kind: 'public-to-connected',
        connection: connection(),
        repository: repository({ id: '', cloneUrl: '' }),
      },
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.code, 'repository_missing')
  })
})

describe('source update safety', () => {
  it('does not mutate inputs or return credential fields', () => {
    const current = snapshot()
    const selected = connection()
    const repo = repository()
    const before = {
      current: structuredClone(current),
      connection: structuredClone(selected),
      repository: structuredClone(repo),
    }
    const config = okConfig(
      buildApplicationSourceUpdate({
        current,
        organizationId: ORG,
        edit: { kind: 'connected-retain', connection: selected, repository: repo, gitBranch: 'release' },
      }),
    )
    assert.deepEqual(current, before.current)
    assert.deepEqual(selected, before.connection)
    assert.deepEqual(repo, before.repository)
    const encoded = JSON.stringify(config)
    for (const field of ['accessToken', 'token', 'password', 'privateKey', 'webhookSecret']) {
      assert.equal(encoded.includes(field), false)
    }
    assert.equal(config.healthCheck === current.healthCheck, false)
    assert.equal(config.runtimeConfig === current.runtimeConfig, false)
  })

  it('rejects a credential-bearing public URL without echoing it', () => {
    const result = buildApplicationSourceUpdate({
      current: snapshot(),
      organizationId: ORG,
      edit: {
        kind: 'connected-to-public',
        repositoryUrl: `https://user:${TOKEN}@github.com/acme/public.git`,
        gitBranch: 'main',
      },
    })
    assert.equal(result.ok, false)
    if (!result.ok) {
      assert.equal(JSON.stringify(result).includes(TOKEN), false)
      assert.equal('config' in result, false)
    }
  })
})
