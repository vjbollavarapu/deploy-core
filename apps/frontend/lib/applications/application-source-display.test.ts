import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  acceptSourceResolution,
  applicationSourceRows,
  classifyApplicationSource,
  connectedSourceMatch,
  IDLE_GIT_METADATA,
  SOURCE_UNAVAILABLE,
  type ApplicationSourceFacts,
  type ConnectedGitDisplayMetadata,
} from './application-source-display'
import type { GitSourceConnection } from './git-source'

const ORG = 'org-a'
const OTHER_ORG = 'org-b'
const APP_ID = '550e8400-e29b-41d4-a716-446655440000'
const OTHER_APP = '550e8400-e29b-41d4-a716-446655440099'
const CONNECTION_ID = '550e8400-e29b-41d4-a716-446655440001'
const OTHER_CONNECTION = '550e8400-e29b-41d4-a716-446655440002'
const REPO_URL = 'https://github.com/acme/app.git'
const TOKEN = 'ghp_secret_token'

function facts(overrides: Partial<ApplicationSourceFacts> = {}): ApplicationSourceFacts {
  return {
    sourceType: 'git',
    repositoryUrl: REPO_URL,
    gitBranch: 'main',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    imageReference: null,
    gitConnectionId: null,
    ...overrides,
  }
}

function connection(overrides: Partial<GitSourceConnection & { account?: string }> = {}) {
  return {
    id: CONNECTION_ID,
    organizationId: ORG,
    provider: 'github',
    type: 'GitHub',
    authMode: 'github_app',
    providerStatus: 'active',
    account: 'acme',
    ...overrides,
  }
}

function repository(overrides: Partial<{ id: string; connectionId: string; organizationId: string; fullName: string; cloneUrl: string }> = {}) {
  return {
    id: 'repo-1',
    connectionId: CONNECTION_ID,
    organizationId: ORG,
    fullName: 'acme/app',
    cloneUrl: REPO_URL,
    ...overrides,
  }
}

function resolved(overrides: Partial<ConnectedGitDisplayMetadata> = {}): ConnectedGitDisplayMetadata {
  return {
    status: 'resolved',
    connectionLabel: 'GitHub · acme · GitHub App',
    repositoryLabel: 'acme/app',
    ...overrides,
  }
}

function row(rows: { label: string; text: string; href: string | null }[], label: string) {
  const match = rows.find((item) => item.label === label)
  assert.ok(match, label)
  return match
}

describe('application source classification', () => {
  it('classifies connected Git only when a connection id is present', () => {
    assert.equal(classifyApplicationSource(facts({ gitConnectionId: CONNECTION_ID })), 'connected-git')
    const rows = applicationSourceRows(facts({ gitConnectionId: CONNECTION_ID }), resolved(), 'summary')
    assert.equal(row(rows, 'Source').text, 'Git')
    assert.equal(row(rows, 'Repository source').text, 'Connected repository')
    assert.equal(row(rows, 'Repository').text, 'acme/app')
    assert.equal(row(rows, 'Repository').href, null)
  })

  it('classifies public Git when no connection id is stored', () => {
    assert.equal(classifyApplicationSource(facts()), 'public-git')
    const rows = applicationSourceRows(facts(), IDLE_GIT_METADATA, 'summary')
    assert.equal(row(rows, 'Repository source').text, 'Public Git')
  })

  it('classifies image and compose without using the repository URL', () => {
    assert.equal(classifyApplicationSource(facts({ sourceType: 'image', repositoryUrl: REPO_URL, gitConnectionId: CONNECTION_ID })), 'image')
    assert.equal(classifyApplicationSource(facts({ sourceType: 'compose', repositoryUrl: REPO_URL })), 'compose')
  })
})

describe('connected Git display', () => {
  it('resolves the connection account and repository name', () => {
    const match = connectedSourceMatch({
      organizationId: ORG,
      gitConnectionId: CONNECTION_ID,
      repositoryUrl: REPO_URL,
      connections: [connection()],
      repositories: [repository()],
    })
    assert.equal(match.connectionLabel, 'GitHub · acme · GitHub App')
    assert.equal(match.repositoryLabel, 'acme/app')
    const rows = applicationSourceRows(
      facts({ gitConnectionId: CONNECTION_ID, dockerfilePath: 'deploy/Dockerfile', buildContext: 'services/api' }),
      { status: 'resolved', ...match },
      'detail',
    )
    assert.equal(row(rows, 'Connection').text, 'GitHub · acme · GitHub App')
    assert.equal(row(rows, 'Repository').text, 'acme/app')
    assert.equal(row(rows, 'Branch').text, 'main')
    assert.equal(row(rows, 'Dockerfile').text, 'deploy/Dockerfile')
    assert.equal(row(rows, 'Build context').text, 'services/api')
    assert.equal(JSON.stringify(rows).includes(REPO_URL), false)
  })

  it('stays Connected Git when the connection or repository is unavailable', () => {
    const missing = connectedSourceMatch({
      organizationId: ORG,
      gitConnectionId: CONNECTION_ID,
      repositoryUrl: REPO_URL,
      connections: [],
      repositories: [],
    })
    assert.equal(missing.connectionLabel, null)
    assert.equal(missing.repositoryLabel, null)
    const rows = applicationSourceRows(
      facts({ gitConnectionId: CONNECTION_ID }),
      { status: 'unavailable', connectionLabel: null, repositoryLabel: null },
      'detail',
    )
    assert.equal(row(rows, 'Repository source').text, 'Connected repository')
    assert.equal(row(rows, 'Connection').text, SOURCE_UNAVAILABLE)
    assert.equal(row(rows, 'Repository').text, SOURCE_UNAVAILABLE)
    assert.equal(JSON.stringify(rows).includes('Public Git'), false)
  })

  it('keeps the connection label when the repository cannot be matched', () => {
    const match = connectedSourceMatch({
      organizationId: ORG,
      gitConnectionId: CONNECTION_ID,
      repositoryUrl: REPO_URL,
      connections: [connection()],
      repositories: [repository({ connectionId: OTHER_CONNECTION, fullName: 'other/repo' })],
    })
    assert.equal(match.connectionLabel, 'GitHub · acme · GitHub App')
    assert.equal(match.repositoryLabel, null)
  })

  it('does not display an unsafe clone URL', () => {
    const unsafe = `https://x-access-token:${TOKEN}@github.com/acme/app.git`
    const match = connectedSourceMatch({
      organizationId: ORG,
      gitConnectionId: CONNECTION_ID,
      repositoryUrl: unsafe,
      connections: [connection()],
      repositories: [repository({ cloneUrl: unsafe })],
    })
    assert.equal(match.repositoryLabel, null)
    const rows = applicationSourceRows(
      facts({ gitConnectionId: CONNECTION_ID, repositoryUrl: unsafe }),
      { status: 'resolved', connectionLabel: match.connectionLabel, repositoryLabel: match.repositoryLabel },
      'detail',
    )
    assert.equal(row(rows, 'Repository').text, SOURCE_UNAVAILABLE)
    assert.equal(row(rows, 'Repository').href, null)
    assert.equal(JSON.stringify(rows).includes(TOKEN), false)
  })

  it('does not display a connection from another organization', () => {
    const match = connectedSourceMatch({
      organizationId: ORG,
      gitConnectionId: CONNECTION_ID,
      repositoryUrl: REPO_URL,
      connections: [connection({ organizationId: OTHER_ORG, account: 'foreign' })],
      repositories: [repository({ organizationId: OTHER_ORG, fullName: 'foreign/app' })],
    })
    assert.equal(match.connectionLabel, null)
    assert.equal(match.repositoryLabel, null)
  })
})

describe('public, image, and compose display', () => {
  it('links a credential-free public repository URL', () => {
    const rows = applicationSourceRows(facts(), IDLE_GIT_METADATA, 'detail')
    assert.equal(row(rows, 'Repository').text, REPO_URL)
    assert.equal(row(rows, 'Repository').href, REPO_URL)
    assert.equal(row(rows, 'Dockerfile').text, 'Dockerfile')
    assert.equal(row(rows, 'Build context').text, '.')
  })

  it('does not link a credential-bearing public URL', () => {
    const unsafe = `https://user:${TOKEN}@github.com/acme/app.git`
    const rows = applicationSourceRows(facts({ repositoryUrl: unsafe }), IDLE_GIT_METADATA, 'summary')
    assert.equal(row(rows, 'Repository source').text, 'Public Git')
    assert.equal(row(rows, 'Repository').text, SOURCE_UNAVAILABLE)
    assert.equal(row(rows, 'Repository').href, null)
    assert.equal(JSON.stringify(rows).includes(TOKEN), false)
  })

  it('hides a schemeless repository string that carries a credential query', () => {
    const cases = [
      `github.com/acme/app?token=${TOKEN}`,
      `github.com/acme/app?access_token=${TOKEN}`,
      `github.com/acme/repo?password=${TOKEN}`,
      `github.com/acme/repo?credential=${TOKEN}`,
      `https://example.com/repo.git?token=${TOKEN}`,
    ]
    for (const repositoryUrl of cases) {
      const rows = applicationSourceRows(facts({ repositoryUrl }), IDLE_GIT_METADATA, 'summary')
      const repository = row(rows, 'Repository')
      assert.equal(repository.text, SOURCE_UNAVAILABLE)
      assert.equal(repository.href, null)
      assert.equal(JSON.stringify(rows).includes(TOKEN), false)
      assert.equal(JSON.stringify(rows).includes(repositoryUrl), false)
    }
  })

  it('shows a schemeless repository string that has no credential query', () => {
    const rows = applicationSourceRows(facts({ repositoryUrl: 'github.com/acme/app' }), IDLE_GIT_METADATA, 'summary')
    assert.equal(row(rows, 'Repository').text, 'github.com/acme/app')
    assert.equal(row(rows, 'Repository').href, null)
  })

  it('shows the image reference and not a stored repository URL', () => {
    const rows = applicationSourceRows(
      facts({
        sourceType: 'image',
        imageReference: 'ghcr.io/acme/app:4',
        repositoryUrl: REPO_URL,
        gitConnectionId: null,
      }),
      IDLE_GIT_METADATA,
      'detail',
    )
    assert.equal(row(rows, 'Source').text, 'Image')
    assert.equal(row(rows, 'Image').text, 'ghcr.io/acme/app:4')
    assert.equal(rows.some((item) => item.label === 'Repository'), false)
    assert.equal(JSON.stringify(rows).includes(REPO_URL), false)
    assert.equal(rows.some((item) => item.text === 'Connected repository'), false)
  })

  it('identifies Compose and keeps its repository and branch', () => {
    const rows = applicationSourceRows(
      facts({
        sourceType: 'compose',
        repositoryUrl: 'https://github.com/acme/compose.git',
        gitBranch: 'main',
        buildContext: 'deploy',
      }),
      IDLE_GIT_METADATA,
      'detail',
    )
    assert.equal(row(rows, 'Source').text, 'Compose')
    assert.equal(row(rows, 'Repository').text, 'https://github.com/acme/compose.git')
    assert.equal(row(rows, 'Branch').text, 'main')
    assert.equal(row(rows, 'Build context').text, 'deploy')
    assert.equal(rows.some((item) => item.text === 'Connected repository'), false)
  })

  it('hides a Compose repository string that carries a credential query', () => {
    const repositoryUrl = `github.com/acme/compose?token=${TOKEN}`
    const rows = applicationSourceRows(
      facts({ sourceType: 'compose', repositoryUrl, gitBranch: 'main' }),
      IDLE_GIT_METADATA,
      'detail',
    )
    assert.equal(row(rows, 'Source').text, 'Compose')
    assert.equal(row(rows, 'Repository').text, SOURCE_UNAVAILABLE)
    assert.equal(row(rows, 'Repository').href, null)
    assert.equal(row(rows, 'Branch').text, 'main')
    assert.equal(JSON.stringify(rows).includes(TOKEN), false)
    assert.equal(JSON.stringify(rows).includes(repositoryUrl), false)
  })
})

describe('source resolution scope', () => {
  const current = {
    organizationId: ORG,
    applicationId: APP_ID,
    gitConnectionId: CONNECTION_ID,
    repositoryUrl: REPO_URL,
  }

  it('rejects a result from another organization, application, or repository', () => {
    assert.equal(acceptSourceResolution(current, current), true)
    assert.equal(acceptSourceResolution({ ...current, organizationId: OTHER_ORG }, current), false)
    assert.equal(acceptSourceResolution({ ...current, applicationId: OTHER_APP }, current), false)
    assert.equal(acceptSourceResolution({ ...current, repositoryUrl: 'https://github.com/acme/other.git' }, current), false)
  })
})

describe('source display security', () => {
  it('returns only label, text, and href', () => {
    const rows = applicationSourceRows(facts({ gitConnectionId: CONNECTION_ID }), resolved(), 'detail')
    for (const item of rows) {
      assert.deepEqual(Object.keys(item).sort(), ['href', 'label', 'text'])
    }
    const encoded = JSON.stringify(rows)
    for (const secret of ['accessToken', 'privateKey', 'webhookSecret', TOKEN]) {
      assert.equal(encoded.includes(secret), false)
    }
  })
})
