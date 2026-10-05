import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { createApplicationWithVariables, type FlowClient } from './create-application-flow'
import {
  wizardCreateSourceConfig,
  wizardDisplayedGitSource,
  wizardGitReviewRows,
  type WizardConnectionOption,
  type WizardCreateSourceInput,
  type WizardRepositoryOption,
} from './git-source-wizard'

const ORG = 'org-a'
const OTHER_ORG = 'org-b'
const APP_ID = '550e8400-e29b-41d4-a716-446655440000'
const PAT_ID = '550e8400-e29b-41d4-a716-446655440001'
const REPO_ID = '550e8400-e29b-41d4-a716-446655440010'
const SAFE_URL = 'https://github.com/acme/app.git'
const TOKEN = 'ghp_should_not_surface'
const UNSAFE_URL = `https://x-access-token:${TOKEN}@github.com/acme/app.git`

function connection(overrides: Partial<WizardConnectionOption> = {}): WizardConnectionOption {
  return {
    id: APP_ID,
    account: 'acme',
    organizationId: ORG,
    authMode: 'github_app',
    providerStatus: 'active',
    type: 'GitHub',
    provider: 'github',
    selectable: true,
    visibility: 'selectable',
    statusLabel: 'Active',
    label: 'GitHub · acme · GitHub App',
    ...overrides,
  }
}

function repository(overrides: Partial<WizardRepositoryOption> = {}): WizardRepositoryOption {
  return {
    id: REPO_ID,
    label: 'acme/app',
    selectable: true,
    selection: {
      id: REPO_ID,
      connectionId: APP_ID,
      organizationId: ORG,
      cloneUrl: SAFE_URL,
      defaultBranch: 'main',
    },
    ...overrides,
  }
}

function input(overrides: Partial<WizardCreateSourceInput> = {}): WizardCreateSourceInput {
  return {
    sourceType: 'git',
    repositorySource: 'connected',
    organizationId: ORG,
    gitConnectionId: APP_ID,
    repositoryId: REPO_ID,
    repositoryUrl: SAFE_URL,
    gitBranch: 'release',
    dockerfilePath: 'Dockerfile.prod',
    buildContext: 'apps/web',
    connections: [connection()],
    repositories: [repository()],
    ...overrides,
  }
}

function reviewFrom(source: WizardCreateSourceInput) {
  return wizardGitReviewRows({
    sourceType: source.sourceType === 'image' ? 'docker-image' : source.sourceType === 'compose' ? 'docker-compose' : source.sourceType,
    repositorySource: source.repositorySource,
    organizationId: source.organizationId,
    gitConnectionId: source.gitConnectionId,
    repositoryId: source.repositoryId,
    repository: source.repositoryUrl ?? '',
    branch: source.gitBranch ?? '',
    dockerfile: source.dockerfilePath ?? '',
    buildContext: source.buildContext ?? '',
    connections: source.connections,
    repositories: source.repositories,
  })
}

describe('connected git create boundary', () => {
  it('sends the connection id, safe repository URL, edited branch, and build files', () => {
    const result = wizardCreateSourceConfig(input({ repositoryUrl: UNSAFE_URL }))
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.sourceType, 'git')
    assert.equal(result.config.gitConnectionId, APP_ID)
    assert.equal(result.config.repositoryUrl, SAFE_URL)
    assert.equal(result.config.gitBranch, 'release')
    assert.equal(result.config.dockerfilePath, 'Dockerfile.prod')
    assert.equal(result.config.buildContext, 'apps/web')
    assert.equal('clearGitConnection' in result.config, false)
    assert.equal(JSON.stringify(result).includes(TOKEN), false)
  })

  it('blocks a malformed connection id without posting', () => {
    const result = wizardCreateSourceConfig(
      input({
        gitConnectionId: 'not-a-uuid',
        connections: [connection({ id: 'not-a-uuid' })],
      }),
    )
    assert.equal(result.ok, false)
    if (result.ok) return
    assert.equal(result.code, 'connection_id')
    assert.equal('config' in result, false)
    assert.equal(result.message.includes(TOKEN), false)
  })

  it('blocks an inactive connection without falling back to public git', () => {
    const result = wizardCreateSourceConfig(
      input({ connections: [connection({ providerStatus: 'disabled', selectable: false })] }),
    )
    assert.equal(result.ok, false)
    if (result.ok) return
    assert.equal(result.code, 'status')
    assert.equal('config' in result, false)
    assert.equal(result.message.includes(SAFE_URL), false)
  })

  it('blocks a connection from another organization', () => {
    const listed = wizardCreateSourceConfig(
      input({ connections: [connection({ organizationId: OTHER_ORG })] }),
    )
    assert.equal(listed.ok, false)
    if (listed.ok) return
    assert.equal(listed.code, 'organization')

    const cleared = wizardCreateSourceConfig(input({ organizationId: OTHER_ORG, connections: [], repositories: [] }))
    assert.equal(cleared.ok, false)
    if (cleared.ok) return
    assert.equal(cleared.code, 'organization')
    assert.equal(JSON.stringify(cleared).includes(APP_ID), false)
  })

  it('blocks a repository from another connection and an unsafe clone URL', () => {
    const mismatch = wizardCreateSourceConfig(
      input({
        repositories: [
          repository({
            selection: {
              id: REPO_ID,
              connectionId: PAT_ID,
              organizationId: ORG,
              cloneUrl: SAFE_URL,
              defaultBranch: 'main',
            },
          }),
        ],
      }),
    )
    assert.equal(mismatch.ok, false)
    if (mismatch.ok) return
    assert.equal(mismatch.code, 'repository_connection')

    const unsafe = wizardCreateSourceConfig(
      input({
        repositoryUrl: UNSAFE_URL,
        repositories: [
          repository({
            selectable: false,
            selection: {
              id: REPO_ID,
              connectionId: APP_ID,
              organizationId: ORG,
              cloneUrl: UNSAFE_URL,
              defaultBranch: 'main',
            },
          }),
        ],
      }),
    )
    assert.equal(unsafe.ok, false)
    if (unsafe.ok) return
    assert.equal(unsafe.code, 'clone_url')
    assert.equal(JSON.stringify(unsafe).includes(TOKEN), false)
    assert.equal(JSON.stringify(unsafe).includes(UNSAFE_URL), false)
  })

  it('does not call POST /applications when connected validation fails', async () => {
    const prepared = wizardCreateSourceConfig(input({ connections: [], repositories: [] }))
    const calls: string[] = []
    const client: FlowClient = {
      async post<T>(path: string): Promise<T> {
        calls.push(path)
        return { application: { id: 'app-1' } } as T
      },
      async get<T>(): Promise<T> {
        return {} as T
      },
    }
    if (prepared.ok) {
      await createApplicationWithVariables(client, {
        organizationId: ORG,
        applicationBody: { config: prepared.config },
        envVars: [],
      })
    }
    assert.equal(prepared.ok, false)
    assert.deepEqual(calls, [])
  })
})

describe('public image and compose create payloads', () => {
  it('sends a public repository URL and omits the connection', () => {
    const result = wizardCreateSourceConfig(
      input({
        repositorySource: 'public',
        repositoryUrl: 'https://github.com/acme/public.git',
        gitBranch: 'main',
        connections: [connection()],
        repositories: [repository()],
      }),
    )
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.sourceType, 'git')
    assert.equal(result.config.repositoryUrl, 'https://github.com/acme/public.git')
    assert.equal(result.config.gitBranch, 'main')
    assert.equal(result.config.dockerfilePath, 'Dockerfile.prod')
    assert.equal(result.config.buildContext, 'apps/web')
    assert.equal('gitConnectionId' in result.config, false)
    assert.equal('clearGitConnection' in result.config, false)
  })

  it('keeps an image payload without a git connection', () => {
    const result = wizardCreateSourceConfig(
      input({
        sourceType: 'image',
        repositorySource: 'connected',
        repositoryUrl: null,
        gitBranch: 'main',
        dockerfilePath: 'Dockerfile',
        buildContext: '.',
      }),
    )
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.sourceType, 'image')
    assert.equal(result.config.repositoryUrl, null)
    assert.equal(result.config.gitBranch, 'main')
    assert.equal(result.config.dockerfilePath, 'Dockerfile')
    assert.equal(result.config.buildContext, '.')
    assert.equal('gitConnectionId' in result.config, false)
    assert.equal('clearGitConnection' in result.config, false)
  })

  it('keeps a compose payload without a git connection', () => {
    const result = wizardCreateSourceConfig(
      input({
        sourceType: 'compose',
        repositorySource: 'connected',
        repositoryUrl: 'https://github.com/acme/stack.git',
        gitBranch: 'main',
      }),
    )
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.sourceType, 'compose')
    assert.equal(result.config.repositoryUrl, 'https://github.com/acme/stack.git')
    assert.equal(result.config.gitBranch, 'main')
    assert.equal('gitConnectionId' in result.config, false)
    assert.equal('clearGitConnection' in result.config, false)
  })
})

describe('create organization isolation', () => {
  it('validates the connection against the current organization', () => {
    const current = wizardCreateSourceConfig(input())
    assert.equal(current.ok, true)
    if (!current.ok) return
    assert.equal(current.config.gitConnectionId, APP_ID)

    const stale = wizardCreateSourceConfig(input({ organizationId: OTHER_ORG }))
    assert.equal(stale.ok, false)
    if (stale.ok) return
    assert.equal(stale.code, 'organization')
    assert.equal('config' in stale, false)
  })
})

describe('git review summary', () => {
  it('shows connected metadata without the clone URL', () => {
    const rows = wizardGitReviewRows({
      sourceType: 'git',
      repositorySource: 'connected',
      organizationId: ORG,
      gitConnectionId: APP_ID,
      repositoryId: REPO_ID,
      repository: UNSAFE_URL,
      branch: 'release',
      dockerfile: 'Dockerfile.prod',
      buildContext: 'apps/web',
      connections: [connection()],
      repositories: [repository()],
    })
    assert.deepEqual(
      rows.map((row) => row.label),
      ['Repository source', 'Connection', 'Repository', 'Branch', 'Dockerfile', 'Build context'],
    )
    assert.equal(rows.find((row) => row.label === 'Repository source')?.value, 'Connected repository')
    assert.equal(rows.find((row) => row.label === 'Connection')?.value, 'GitHub · acme · GitHub App')
    assert.equal(rows.find((row) => row.label === 'Repository')?.value, 'acme/app')
    assert.equal(rows.find((row) => row.label === 'Branch')?.value, 'release')
    const rendered = JSON.stringify(rows)
    assert.equal(rendered.includes(SAFE_URL), false)
    assert.equal(rendered.includes(UNSAFE_URL), false)
    assert.equal(rendered.includes(TOKEN), false)
    const source = wizardDisplayedGitSource({
      sourceType: 'git',
      repositorySource: 'connected',
      organizationId: ORG,
      gitConnectionId: APP_ID,
      repositoryId: REPO_ID,
      repository: UNSAFE_URL,
      branch: 'release',
      dockerfile: 'Dockerfile.prod',
      buildContext: 'apps/web',
      connections: [connection()],
      repositories: [repository()],
    })
    assert.equal(source, 'acme/app:release')
    assert.equal(source.includes('github.com'), false)
  })

  it('shows an unavailable state when connected metadata is gone', () => {
    const rows = reviewFrom(input({ connections: [], repositories: [], repositoryUrl: UNSAFE_URL }))
    assert.equal(rows.find((row) => row.label === 'Connection')?.value, 'Unavailable')
    assert.equal(rows.find((row) => row.label === 'Repository')?.value, 'Unavailable')
    assert.equal(JSON.stringify(rows).includes(UNSAFE_URL), false)
    assert.equal(JSON.stringify(rows).includes(TOKEN), false)
  })

  it('shows the public repository URL', () => {
    const rows = reviewFrom(
      input({
        repositorySource: 'public',
        repositoryUrl: 'https://github.com/acme/public.git',
        gitBranch: 'main',
      }),
    )
    assert.equal(rows.find((row) => row.label === 'Repository source')?.value, 'Public Git URL')
    assert.equal(rows.find((row) => row.label === 'Repository')?.value, 'https://github.com/acme/public.git')
    assert.equal(rows.some((row) => row.label === 'Connection'), false)
    assert.equal(JSON.stringify(rows).includes(TOKEN), false)
  })
})

describe('connected create bootstrap', () => {
  it('posts the application and does not create a deployment', async () => {
    const prepared = wizardCreateSourceConfig(input())
    assert.equal(prepared.ok, true)
    if (!prepared.ok) return
    const calls: Array<{ path: string; body: unknown }> = []
    const client: FlowClient = {
      async post<T>(path: string, body?: unknown): Promise<T> {
        calls.push({ path, body })
        if (path.includes('/deployments')) throw new Error('deployment must not be queued')
        return { application: { id: 'app-1' } } as T
      },
      async get<T>(): Promise<T> {
        return {} as T
      },
    }
    const created = await createApplicationWithVariables(client, {
      organizationId: ORG,
      applicationBody: { organizationId: ORG, config: prepared.config },
      envVars: [],
    })
    assert.equal(created.applicationId, 'app-1')
    assert.deepEqual(
      calls.map((call) => call.path),
      ['/applications'],
    )
    const posted = calls[0].body as { config: { gitConnectionId?: string; repositoryUrl?: string; gitBranch?: string } }
    assert.equal(posted.config.gitConnectionId, APP_ID)
    assert.equal(posted.config.repositoryUrl, SAFE_URL)
    assert.equal(posted.config.gitBranch, 'release')
    assert.equal(JSON.stringify(calls).includes(TOKEN), false)
  })
})
