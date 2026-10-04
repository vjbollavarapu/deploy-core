import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { collectRepositoryPages } from '../github/providers'
import {
  classifyGitConnections,
  collectGitConnectionPages,
  fieldsAfterConnectionChange,
  credentialFreeCloneUrl,
  fieldsAfterLeavingGit,
  fieldsAfterOrganizationChange,
  fieldsAfterPublicGit,
  fieldsAfterRepositorySelection,
  gitConnectionEligibility,
  gitConnectionListRequest,
  gitSourceCreateConfig,
  resetPlacementOnOrganizationChange,
  selectConnectedRepository,
  validateConnectedGitSubmission,
  WIZARD_REPOSITORY_PAGE_LIMIT,
  type GitSourceConnection,
  type GitSourceRepository,
} from './git-source'

const ORG = 'org-a'
const APP_ID = '550e8400-e29b-41d4-a716-446655440000'
const PAT_ID = '550e8400-e29b-41d4-a716-446655440001'

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
    cloneUrl: 'https://github.com/acme/app.git',
    defaultBranch: 'main',
    ...overrides,
  }
}

describe('connection eligibility', () => {
  it('accepts an active GitHub App connection', () => {
    const result = gitConnectionEligibility(connection(), ORG)
    assert.equal(result.selectable, true)
    assert.equal(result.visibility, 'selectable')
    assert.equal(result.statusLabel, 'Active')
    assert.equal(result.authModeLabel, 'GitHub App')
  })

  it('accepts an active PAT connection', () => {
    const result = gitConnectionEligibility(connection({ id: PAT_ID, authMode: 'pat' }), ORG)
    assert.equal(result.selectable, true)
    assert.equal(result.authModeLabel, 'Personal access token')
  })

  it('sorts a GitHub App connection before a PAT connection without mutating the source', () => {
    const pat = connection({ id: PAT_ID, authMode: 'pat' })
    const gitlab = connection({ id: 'gl-1', provider: 'gitlab', authMode: 'pat' })
    const app = connection()
    const source = [pat, gitlab, app]
    const frozen = source.map((item) => item.id)
    const classified = classifyGitConnections(source, ORG)
    assert.deepEqual(
      classified.map((item) => item.connection.id),
      [APP_ID, PAT_ID, 'gl-1'],
    )
    assert.deepEqual(source.map((item) => item.id), frozen)
    assert.equal(classified[0]?.eligibility.selectable, true)
    assert.equal(classified[1]?.eligibility.selectable, true)
    assert.equal(classified[2]?.eligibility.selectable, false)
    assert.equal(classified[2]?.eligibility.visibility, 'hidden')
  })

  it('keeps disabled, error, revoked, and unknown GitHub connections visible and unselectable', () => {
    for (const providerStatus of ['disabled', 'error', 'revoked', 'paused']) {
      const result = gitConnectionEligibility(connection({ providerStatus }), ORG)
      assert.equal(result.selectable, false)
      assert.equal(result.visibility, 'unavailable')
    }
    assert.equal(gitConnectionEligibility(connection({ providerStatus: 'disabled' }), ORG).statusLabel, 'Suspended')
    assert.equal(gitConnectionEligibility(connection({ providerStatus: 'error' }), ORG).statusLabel, 'Error')
    assert.equal(gitConnectionEligibility(connection({ providerStatus: 'revoked' }), ORG).statusLabel, 'Revoked')
    assert.equal(gitConnectionEligibility(connection({ providerStatus: 'paused' }), ORG).statusLabel, 'Unknown')
  })

  it('hides a non-GitHub provider and a connection from another organization', () => {
    for (const provider of ['gitlab', 'bitbucket', 'generic']) {
      const result = gitConnectionEligibility(connection({ provider }), ORG)
      assert.equal(result.selectable, false)
      assert.equal(result.visibility, 'hidden')
    }
    const otherOrg = gitConnectionEligibility(connection({ organizationId: 'org-b' }), ORG)
    assert.equal(otherOrg.selectable, false)
    assert.equal(otherOrg.visibility, 'hidden')
  })
})

describe('repository selection', () => {
  it('returns a credential-free clone URL and uses main when the default branch is empty', () => {
    const selected = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ defaultBranch: '  ' }),
    })
    assert.equal(selected.ok, true)
    if (!selected.ok) return
    assert.equal(selected.repositoryUrl, 'https://github.com/acme/app.git')
    assert.equal(selected.branch, 'main')
    assert.equal(JSON.stringify(selected).includes('token'), false)
  })

  it('rejects a repository from another connection, another organization, or an unsafe URL', () => {
    const wrongConnection = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ connectionId: PAT_ID }),
    })
    assert.deepEqual(wrongConnection, {
      ok: false,
      code: 'repository_connection',
      message: 'Choose a repository from the selected connection.',
    })

    const wrongOrg = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ organizationId: 'org-b' }),
    })
    assert.equal(wrongOrg.ok, false)
    if (!wrongOrg.ok) assert.equal(wrongOrg.code, 'repository_organization')

    const token = 'ghp_should_not_surface'
    const userinfo = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ cloneUrl: `https://x-access-token:${token}@github.com/acme/app.git` }),
    })
    assert.equal(userinfo.ok, false)
    assert.equal(JSON.stringify(userinfo).includes(token), false)

    const javascriptUrl = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ cloneUrl: 'javascript:alert(1)' }),
    })
    assert.equal(javascriptUrl.ok, false)
    if (!javascriptUrl.ok) assert.equal(javascriptUrl.code, 'clone_url')

    const malformed = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ cloneUrl: 'not a url' }),
    })
    assert.equal(malformed.ok, false)
    if (!malformed.ok) assert.equal(malformed.code, 'clone_url')
  })

  it('replaces the branch when the repository changes', () => {
    const current = {
      gitConnectionId: APP_ID,
      repositoryId: 'repo-1',
      repositoryUrl: 'https://github.com/acme/app.git',
      branch: 'feature',
      dockerfile: 'Dockerfile.prod',
      buildContext: 'apps/backend',
    }
    const next = fieldsAfterRepositorySelection(current, {
      repositoryId: 'repo-2',
      repositoryUrl: 'https://github.com/acme/other.git',
      branch: 'trunk',
    })
    assert.equal(next.repositoryId, 'repo-2')
    assert.equal(next.repositoryUrl, 'https://github.com/acme/other.git')
    assert.equal(next.branch, 'trunk')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'apps/backend')
  })

  it('clears the repository when the connection or organization changes', () => {
    const current = {
      gitConnectionId: APP_ID,
      repositoryId: 'repo-1',
      repositoryUrl: 'https://github.com/acme/app.git',
      branch: 'feature',
      dockerfile: 'Dockerfile.prod',
      buildContext: 'apps/backend',
    }
    const connectionChange = fieldsAfterConnectionChange(current, PAT_ID)
    assert.deepEqual(
      {
        gitConnectionId: connectionChange.gitConnectionId,
        repositoryId: connectionChange.repositoryId,
        repositoryUrl: connectionChange.repositoryUrl,
        branch: connectionChange.branch,
      },
      { gitConnectionId: PAT_ID, repositoryId: '', repositoryUrl: '', branch: 'main' },
    )
    assert.equal(connectionChange.dockerfile, 'Dockerfile.prod')

    const organizationChange = fieldsAfterOrganizationChange(current)
    assert.equal(organizationChange.gitConnectionId, '')
    assert.equal(organizationChange.repositoryId, '')
    assert.equal(organizationChange.repositoryUrl, '')
    assert.equal(organizationChange.branch, 'main')
    assert.equal(organizationChange.buildContext, 'apps/backend')
  })

  it('clears connected Git state and keeps Dockerfile and build context when the source leaves Git', () => {
    const current = {
      gitConnectionId: APP_ID,
      repositoryId: 'repo-1',
      repositoryUrl: 'https://github.com/acme/app.git',
      branch: 'feature',
      dockerfile: 'Dockerfile.prod',
      buildContext: 'apps/backend',
    }
    const next = fieldsAfterLeavingGit(current)
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.repositoryUrl, '')
    assert.equal(next.branch, 'feature')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'apps/backend')
  })
})

describe('connection pagination', () => {
  it('requests the organization id and does not stop after the default 20 rows', async () => {
    const calls: Array<{ organizationId: string; limit: number; offset: number }> = []
    const items = await collectGitConnectionPages(ORG, async (page) => {
      calls.push(page)
      const start = page.offset
      const slice = Array.from({ length: Math.min(page.limit, Math.max(0, 25 - start)) }, (_, index) => start + index)
      return { items: slice, totalCount: 25 }
    })
    assert.equal(items.length, 25)
    assert.deepEqual(calls, [{ organizationId: ORG, limit: WIZARD_REPOSITORY_PAGE_LIMIT, offset: 0 }])
    assert.equal(gitConnectionListRequest(ORG).path, `/integrations/git/connections?organizationId=${ORG}`)
    assert.equal(
      gitConnectionListRequest(ORG, { limit: 100, offset: 100 }).path,
      `/integrations/git/connections?organizationId=${ORG}&limit=100&offset=100`,
    )
  })

  it('loads one short page, several pages, and stops on an empty page', async () => {
    const onePage = await collectGitConnectionPages('org-one', async (page) => {
      assert.equal(page.organizationId, 'org-one')
      assert.equal(page.limit, 100)
      return { items: ['only'], totalCount: 1 }
    })
    assert.deepEqual(onePage, ['only'])

    const calls: number[] = []
    const many = await collectGitConnectionPages(ORG, async (page) => {
      calls.push(page.offset)
      assert.equal(page.organizationId, ORG)
      const start = page.offset
      const slice = Array.from({ length: Math.min(page.limit, Math.max(0, 150 - start)) }, (_, index) => start + index)
      return { items: slice, totalCount: 150 }
    })
    assert.equal(many.length, 150)
    assert.deepEqual(calls, [0, 100])

    let emptyCalls = 0
    const empty = await collectGitConnectionPages(ORG, async () => {
      emptyCalls += 1
      return { items: [] }
    })
    assert.deepEqual(empty, [])
    assert.equal(emptyCalls, 1)
  })

  it('stops when every page is full and totalCount is absent', async () => {
    let calls = 0
    const items = await collectGitConnectionPages(ORG, async (page) => {
      calls += 1
      assert.equal(page.organizationId, ORG)
      assert.equal(page.limit, 100)
      return { items: Array.from({ length: page.limit }, () => 'row') }
    })
    assert.equal(calls, 100)
    assert.equal(items.length, 10000)
  })
})

describe('repository pagination reuse', () => {
  it('loads wizard repositories with the existing page collector', async () => {
    const calls: Array<{ limit: number; offset: number }> = []
    const items = await collectRepositoryPages(async (page) => {
      calls.push(page)
      const start = page.offset
      const slice = Array.from({ length: Math.min(page.limit, Math.max(0, 101 - start)) }, (_, index) => start + index)
      return { items: slice, totalCount: 101 }
    })
    assert.equal(WIZARD_REPOSITORY_PAGE_LIMIT, 100)
    assert.equal(items.length, 101)
    assert.deepEqual(calls, [
      { limit: 100, offset: 0 },
      { limit: 100, offset: 100 },
    ])
  })
})

describe('create payload', () => {
  const connectedInput = {
    repositorySource: 'connected' as const,
    organizationId: ORG,
    connection: connection(),
    repository: repository(),
  }

  it('includes the connection id and safe repository URL for connected Git', () => {
    const token = 'ghp_should_not_surface'
    const result = gitSourceCreateConfig(
      {
        sourceType: 'git',
        repositoryUrl: `https://x-access-token:${token}@github.com/acme/app.git`,
        gitBranch: 'main',
        dockerfilePath: 'Dockerfile.prod',
        buildContext: 'apps/backend',
        clearGitConnection: true,
      },
      connectedInput,
    )
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.gitConnectionId, APP_ID)
    assert.equal(result.config.repositoryUrl, 'https://github.com/acme/app.git')
    assert.equal(result.config.dockerfilePath, 'Dockerfile.prod')
    assert.equal(result.config.buildContext, 'apps/backend')
    assert.equal('clearGitConnection' in result.config, false)
    assert.equal(JSON.stringify(result).includes(token), false)
  })

  it('omits the connection id for public Git, image, and Compose', () => {
    const stale = {
      gitConnectionId: APP_ID,
      clearGitConnection: true,
      gitBranch: 'main',
      dockerfilePath: 'Dockerfile',
      buildContext: '.',
    }
    const publicGit = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: 'github.com/acme/app', ...stale },
      { repositorySource: 'public' },
    )
    assert.equal(publicGit.ok, true)
    if (!publicGit.ok) return
    assert.equal(publicGit.config.repositoryUrl, 'github.com/acme/app')
    assert.equal('gitConnectionId' in publicGit.config, false)
    assert.equal('clearGitConnection' in publicGit.config, false)

    const image = gitSourceCreateConfig(
      { sourceType: 'image', repositoryUrl: 'github.com/acme/app', ...stale },
      connectedInput,
    )
    assert.equal(image.ok, true)
    if (!image.ok) return
    assert.equal('gitConnectionId' in image.config, false)
    assert.equal('clearGitConnection' in image.config, false)
    assert.equal(image.config.gitBranch, 'main')
    assert.equal(image.config.dockerfilePath, 'Dockerfile')

    const compose = gitSourceCreateConfig(
      { sourceType: 'compose', repositoryUrl: 'github.com/acme/app', ...stale },
      connectedInput,
    )
    assert.equal(compose.ok, true)
    if (!compose.ok) return
    assert.equal('gitConnectionId' in compose.config, false)
    assert.equal('clearGitConnection' in compose.config, false)

    const formImage = gitSourceCreateConfig(
      { sourceType: 'docker-image', ...stale },
      connectedInput,
    )
    assert.equal(formImage.ok, true)
    if (!formImage.ok) return
    assert.equal('gitConnectionId' in formImage.config, false)
    const formCompose = gitSourceCreateConfig(
      { sourceType: 'docker-compose', ...stale },
      connectedInput,
    )
    assert.equal(formCompose.ok, true)
    if (!formCompose.ok) return
    assert.equal('gitConnectionId' in formCompose.config, false)
  })
})

describe('connected git fail closed', () => {
  const privateUrl = 'https://github.com/acme/private.git'
  const connectedState = {
    gitConnectionId: APP_ID,
    repositoryId: 'repo-1',
    repositoryUrl: privateUrl,
    branch: 'feature',
    dockerfile: 'Dockerfile.prod',
    buildContext: 'apps/backend',
    name: 'api',
  }

  function closed(
    result: { ok: boolean; code?: string; message?: string; config?: { repositoryUrl?: string | null; gitConnectionId?: string } },
    code: string,
  ) {
    assert.equal(result.ok, false)
    if (result.ok) return
    assert.equal(result.code, code)
    assert.equal('config' in result, false)
    assert.equal(JSON.stringify(result).includes(privateUrl), false)
    assert.equal(JSON.stringify(result).includes('SECRET'), false)
    assert.equal(JSON.stringify(result).includes('ghp_'), false)
  }

  it('clears the derived URL when connected Git switches to public Git', () => {
    const next = fieldsAfterPublicGit(connectedState)
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.repositoryUrl, '')
    assert.equal(next.branch, 'feature')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'apps/backend')
    assert.equal(next.name, 'api')
    const created = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: next.repositoryUrl, gitBranch: next.branch, clearGitConnection: true },
      { repositorySource: 'public' },
    )
    assert.equal(created.ok, true)
    if (!created.ok) return
    assert.equal(created.config.repositoryUrl, '')
    assert.equal('gitConnectionId' in created.config, false)
    assert.equal('clearGitConnection' in created.config, false)
  })

  it('fails closed for a malformed connection id', () => {
    const result = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: privateUrl, clearGitConnection: true },
      {
        repositorySource: 'connected',
        organizationId: ORG,
        connection: connection({ id: 'not-a-uuid' }),
        repository: repository({ connectionId: 'not-a-uuid', cloneUrl: privateUrl }),
      },
    )
    closed(result, 'connection_id')
  })

  it('fails closed for an inactive connection', () => {
    const result = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: privateUrl },
      {
        repositorySource: 'connected',
        organizationId: ORG,
        connection: connection({ providerStatus: 'disabled' }),
        repository: repository({ cloneUrl: privateUrl }),
      },
    )
    closed(result, 'status')
  })

  it('fails closed for the wrong organization', () => {
    const result = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: privateUrl },
      {
        repositorySource: 'connected',
        organizationId: ORG,
        connection: connection({ organizationId: 'org-b' }),
        repository: repository({ cloneUrl: privateUrl }),
      },
    )
    closed(result, 'organization')
  })

  it('fails closed when the repository belongs to another connection', () => {
    const result = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: privateUrl },
      {
        repositorySource: 'connected',
        organizationId: ORG,
        connection: connection(),
        repository: repository({ connectionId: PAT_ID, cloneUrl: privateUrl }),
      },
    )
    closed(result, 'repository_connection')
  })

  it('fails closed for an unsafe clone URL', () => {
    const result = gitSourceCreateConfig(
      { sourceType: 'git', repositoryUrl: privateUrl },
      {
        repositorySource: 'connected',
        organizationId: ORG,
        connection: connection(),
        repository: repository({ cloneUrl: 'https://user:ghp_secret@github.com/acme/private.git' }),
      },
    )
    closed(result, 'clone_url')
  })

  it('keeps an explicitly entered public repository URL without a connection', () => {
    const result = gitSourceCreateConfig(
      {
        sourceType: 'git',
        repositoryUrl: 'github.com/acme/public',
        gitConnectionId: APP_ID,
        clearGitConnection: true,
        dockerfilePath: 'Dockerfile',
        buildContext: '.',
      },
      { repositorySource: 'public' },
    )
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.config.repositoryUrl, 'github.com/acme/public')
    assert.equal('gitConnectionId' in result.config, false)
    assert.equal('clearGitConnection' in result.config, false)
    assert.equal(result.config.dockerfilePath, 'Dockerfile')
  })

  it('clears the connected URL when the source becomes image or Compose', () => {
    const image = fieldsAfterLeavingGit({ ...connectedState, sourceType: 'docker-image' })
    assert.equal(image.gitConnectionId, '')
    assert.equal(image.repositoryId, '')
    assert.equal(image.repositoryUrl, '')
    assert.equal(image.dockerfile, 'Dockerfile.prod')
    assert.equal(image.buildContext, 'apps/backend')

    const compose = fieldsAfterLeavingGit({ ...connectedState, sourceType: 'docker-compose' })
    assert.equal(compose.gitConnectionId, '')
    assert.equal(compose.repositoryId, '')
    assert.equal(compose.repositoryUrl, '')
    assert.equal(compose.buildContext, 'apps/backend')
  })

  it('clears the connected URL when the organization or connection changes', () => {
    const organization = fieldsAfterOrganizationChange(connectedState)
    assert.equal(organization.gitConnectionId, '')
    assert.equal(organization.repositoryId, '')
    assert.equal(organization.repositoryUrl, '')
    assert.equal(organization.branch, 'main')
    assert.equal(organization.dockerfile, 'Dockerfile.prod')

    const changed = fieldsAfterConnectionChange(connectedState, PAT_ID)
    assert.equal(changed.gitConnectionId, PAT_ID)
    assert.equal(changed.repositoryId, '')
    assert.equal(changed.repositoryUrl, '')
    assert.equal(changed.buildContext, 'apps/backend')
  })
})

describe('clone URL credentials', () => {
  it('rejects a query credential and userinfo, and accepts a normal GitHub clone URL', () => {
    const secret = 'SECRET'
    const query = credentialFreeCloneUrl(`https://github.com/owner/repo.git?access_token=${secret}`)
    assert.equal(query, null)
    assert.equal(credentialFreeCloneUrl('https://user:password@github.com/owner/repo.git'), null)
    assert.equal(credentialFreeCloneUrl('https://github.com/owner/repo.git'), 'https://github.com/owner/repo.git')
    assert.equal(credentialFreeCloneUrl('https://.'), null)
    assert.equal(credentialFreeCloneUrl('https://'), null)
    assert.equal(credentialFreeCloneUrl('not a url'), null)
    const selected = selectConnectedRepository({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ cloneUrl: `https://github.com/owner/repo.git?token=${secret}` }),
    })
    assert.equal(selected.ok, false)
    assert.equal(JSON.stringify(selected).includes(secret), false)
  })
})

describe('submit guard', () => {
  it('accepts an active GitHub App selection and an active PAT selection', () => {
    const app = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection(),
      repository: repository(),
    })
    assert.equal(app.ok, true)
    if (app.ok) assert.equal(app.repositoryUrl, 'https://github.com/acme/app.git')

    const pat = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection({ id: PAT_ID, authMode: 'pat' }),
      repository: repository({ connectionId: PAT_ID }),
    })
    assert.equal(pat.ok, true)
  })

  it('rejects the wrong organization, an inactive connection, a mismatched repository, and an unsafe URL', () => {
    const wrongOrg = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection({ organizationId: 'org-b' }),
      repository: repository(),
    })
    assert.equal(wrongOrg.ok, false)
    if (!wrongOrg.ok) assert.equal(wrongOrg.code, 'organization')

    const inactive = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection({ providerStatus: 'disabled' }),
      repository: repository(),
    })
    assert.equal(inactive.ok, false)
    if (!inactive.ok) assert.equal(inactive.code, 'status')

    const mismatch = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ connectionId: PAT_ID }),
    })
    assert.equal(mismatch.ok, false)
    if (!mismatch.ok) assert.equal(mismatch.code, 'repository_connection')

    const unsafe = validateConnectedGitSubmission({
      organizationId: ORG,
      connection: connection(),
      repository: repository({ cloneUrl: 'https://user:ghp_secret@github.com/acme/app.git' }),
    })
    assert.equal(unsafe.ok, false)
    if (!unsafe.ok) assert.equal(unsafe.code, 'clone_url')
    assert.equal(JSON.stringify(unsafe).includes('ghp_secret'), false)
  })
})

describe('placement organization reset', () => {
  it('clears project, environment, and server and keeps unrelated fields', () => {
    const next = resetPlacementOnOrganizationChange({
      projectId: 'project-a',
      environment: 'env-a',
      environmentId: 'env-a',
      serverId: 'server-a',
      name: 'modulyn',
      branch: 'main',
      dockerfile: 'Dockerfile.prod',
    })
    assert.equal(next.projectId, '')
    assert.equal(next.environment, '')
    assert.equal(next.environmentId, '')
    assert.equal(next.serverId, '')
    assert.equal(next.name, 'modulyn')
    assert.equal(next.branch, 'main')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
  })
})
