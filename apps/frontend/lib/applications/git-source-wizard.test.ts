import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { REPOSITORY_PAGE_LIMIT } from '../github/providers'
import {
  applyConnectedGitTransition,
  applyConnectionChange,
  applyOrganizationChange,
  applyPublicGitTransition,
  applyRepositorySelection,
  applySourceTypeChange,
  connectionListMessage,
  connectionSelectionAfterRefresh,
  defaultRepositorySource,
  loadWizardGitConnections,
  loadWizardRepositories,
  repositoryListMessage,
  commitWizardGitCache,
  connectionSelectionAllowed,
  repositoryLoadResult,
  visibleWizardGitCache,
  wizardConnectionOptions,
  wizardGitCacheAfterConnectionFailure,
  wizardGitCacheAfterConnectionsLoaded,
  wizardGitCacheAfterOrganizationChange,
  wizardGitCacheAfterRepositoriesLoaded,
  wizardRepositoryOptions,
  type WizardConnectionOption,
  type WizardGitCache,
  type RepositoryLoadScope,
  type WizardConnectionInput,
  type WizardGitFormState,
  type WizardRepositoryInput,
} from './git-source-wizard'

const ORG = 'org-a'
const OTHER_ORG = 'org-b'
const APP_ID = '550e8400-e29b-41d4-a716-446655440000'
const PAT_ID = '550e8400-e29b-41d4-a716-446655440001'
const BROKEN_ID = '550e8400-e29b-41d4-a716-446655440002'
const REPO_ID = '550e8400-e29b-41d4-a716-446655440010'
const REPO_ID_2 = '550e8400-e29b-41d4-a716-446655440011'
const SAFE_URL = 'https://github.com/acme/api.git'
const OTHER_URL = 'https://github.com/acme/web.git'

function form(overrides: Partial<WizardGitFormState> = {}): WizardGitFormState {
  return {
    sourceType: 'git',
    repositorySource: 'connected',
    gitConnectionId: '',
    repositoryId: '',
    repository: '',
    branch: 'main',
    dockerfile: 'Dockerfile',
    buildContext: '.',
    projectId: 'project-1',
    environment: 'production',
    serverId: 'server-1',
    ...overrides,
  }
}

function appConnection(overrides: Partial<WizardConnectionInput> = {}): WizardConnectionInput {
  return {
    id: APP_ID,
    organizationId: ORG,
    type: 'GitHub',
    authMode: 'github_app',
    providerStatus: 'active',
    account: 'acme',
    ...overrides,
  }
}

function patConnection(overrides: Partial<WizardConnectionInput> = {}): WizardConnectionInput {
  return {
    id: PAT_ID,
    organizationId: ORG,
    type: 'GitHub',
    authMode: 'pat',
    providerStatus: 'active',
    account: 'pat-user',
    ...overrides,
  }
}

function repository(overrides: Partial<WizardRepositoryInput> = {}): WizardRepositoryInput {
  return {
    id: REPO_ID,
    connectionId: APP_ID,
    organizationId: ORG,
    fullName: 'acme/api',
    defaultBranch: 'develop',
    cloneUrl: SAFE_URL,
    metadata: { private: true },
    ...overrides,
  }
}

function scope(overrides: Partial<RepositoryLoadScope> = {}): RepositoryLoadScope {
  return {
    open: true,
    organizationId: ORG,
    connectionId: APP_ID,
    repositorySource: 'connected',
    sourceType: 'git',
    generation: 1,
    ...overrides,
  }
}

describe('repository source availability', () => {
  it('defaults to connected when a selectable connection exists and the form is untouched', () => {
    assert.equal(
      defaultRepositorySource({
        selectableCount: 1,
        repository: '',
        gitConnectionId: '',
        repositoryId: '',
        modeChosen: false,
        current: 'public',
      }),
      'connected',
    )
  })

  it('keeps public Git usable when no connection can be selected', () => {
    assert.equal(
      defaultRepositorySource({
        selectableCount: 0,
        repository: '',
        gitConnectionId: '',
        repositoryId: '',
        modeChosen: false,
        current: 'public',
      }),
      null,
    )
    assert.equal(connectionListMessage({ phase: 'ready', visibleCount: 0, selectableCount: 0 }), 'No Git connections available')
  })

  it('does not override a chosen mode or a typed public repository', () => {
    assert.equal(
      defaultRepositorySource({
        selectableCount: 2,
        repository: '',
        gitConnectionId: '',
        repositoryId: '',
        modeChosen: true,
        current: 'public',
      }),
      null,
    )
    assert.equal(
      defaultRepositorySource({
        selectableCount: 2,
        repository: 'https://github.com/acme/public.git',
        gitConnectionId: '',
        repositoryId: '',
        modeChosen: false,
        current: 'public',
      }),
      null,
    )
  })

  it('does not leave connected mode when the refreshed list has no selectable connection', () => {
    assert.equal(
      defaultRepositorySource({
        selectableCount: 0,
        repository: '',
        gitConnectionId: '',
        repositoryId: '',
        modeChosen: false,
        current: 'connected',
      }),
      null,
    )
  })
})

describe('wizard connection options', () => {
  const options = wizardConnectionOptions(
    [
      patConnection(),
      appConnection(),
      appConnection({
        id: BROKEN_ID,
        account: 'broken',
        providerStatus: 'error',
        authMode: 'github_app',
      }),
      appConnection({ id: '550e8400-e29b-41d4-a716-446655440003', organizationId: OTHER_ORG, account: 'other' }),
      { id: 'gitlab', organizationId: ORG, provider: 'gitlab', authMode: 'pat', providerStatus: 'active', account: 'gl' },
    ],
    ORG,
  )

  it('shows GitHub App before PAT and keeps both selectable', () => {
    assert.deepEqual(
      options.filter((option) => option.selectable).map((option) => option.authMode),
      ['github_app', 'pat'],
    )
    assert.equal(options[0]?.label, 'GitHub · acme · GitHub App')
    assert.equal(options[1]?.label, 'GitHub · pat-user · Personal access token')
    assert.equal(options[0]?.selectable, true)
    assert.equal(options[1]?.selectable, true)
  })

  it('shows an unavailable GitHub connection without making it selectable', () => {
    const unavailable = options.find((option) => option.id === BROKEN_ID)
    assert.equal(unavailable?.selectable, false)
    assert.equal(unavailable?.visibility, 'unavailable')
    assert.match(unavailable?.label ?? '', /Error/)
  })

  it('hides another organization and a non-GitHub connection', () => {
    assert.equal(options.some((option) => option.organizationId === OTHER_ORG), false)
    assert.equal(options.some((option) => option.id === 'gitlab'), false)
  })

  it('does not copy credential fields into the option', () => {
    const listed = wizardConnectionOptions(
      [
        {
          ...appConnection(),
          accessToken: 'ghp_secret',
          webhookSecret: 'whsec_secret',
        } as WizardConnectionInput,
      ],
      ORG,
    )
    const encoded = JSON.stringify(listed)
    assert.equal(encoded.includes('ghp_secret'), false)
    assert.equal(encoded.includes('whsec_secret'), false)
    assert.equal(encoded.includes('accessToken'), false)
  })
})

describe('wizard repository options', () => {
  const connection = appConnection()

  it('labels owner/repository with visibility and archived metadata', () => {
    const [option] = wizardRepositoryOptions({
      organizationId: ORG,
      connection,
      repositories: [repository({ metadata: { private: true, archived: true } })],
    })
    assert.equal(option?.label, 'acme/api · Private · Archived')
    assert.equal(option?.label.includes('http'), false)
  })

  it('writes the safe clone URL and default branch', () => {
    const [option] = wizardRepositoryOptions({
      organizationId: ORG,
      connection,
      repositories: [repository()],
    })
    assert.ok(option)
    assert.equal(option.selectable, true)
    assert.ok(option.selection)
    const selected = applyRepositorySelection(form({ gitConnectionId: APP_ID, branch: 'feature', dockerfile: 'Dockerfile.prod' }), {
      organizationId: ORG,
      connection,
      repository: option.selection,
    })
    assert.equal(selected.ok, true)
    if (!selected.ok) return
    assert.equal(selected.state.repositoryId, REPO_ID)
    assert.equal(selected.state.repository, SAFE_URL)
    assert.equal(selected.state.branch, 'develop')
    assert.equal(selected.state.dockerfile, 'Dockerfile.prod')
    assert.equal(selected.state.buildContext, '.')
  })

  it('uses main when the default branch is empty', () => {
    const [option] = wizardRepositoryOptions({
      organizationId: ORG,
      connection,
      repositories: [repository({ defaultBranch: '   ' })],
    })
    assert.ok(option)
    assert.ok(option.selection)
    const selected = applyRepositorySelection(form({ gitConnectionId: APP_ID }), {
      organizationId: ORG,
      connection,
      repository: option.selection,
    })
    assert.equal(selected.ok, true)
    if (!selected.ok) return
    assert.equal(selected.state.branch, 'main')
  })

  it('replaces the branch when another repository is selected', () => {
    const first = applyRepositorySelection(form({ gitConnectionId: APP_ID, branch: 'old' }), {
      organizationId: ORG,
      connection,
      repository: {
        id: REPO_ID,
        connectionId: APP_ID,
        organizationId: ORG,
        cloneUrl: SAFE_URL,
        defaultBranch: 'develop',
      },
    })
    assert.equal(first.ok, true)
    if (!first.ok) return
    const second = applyRepositorySelection(first.state, {
      organizationId: ORG,
      connection,
      repository: {
        id: REPO_ID_2,
        connectionId: APP_ID,
        organizationId: ORG,
        cloneUrl: OTHER_URL,
        defaultBranch: 'release',
      },
    })
    assert.equal(second.ok, true)
    if (!second.ok) return
    assert.equal(second.state.repositoryId, REPO_ID_2)
    assert.equal(second.state.repository, OTHER_URL)
    assert.equal(second.state.branch, 'release')
    assert.equal(second.state.dockerfile, 'Dockerfile')
  })

  it('does not select or display a credential-bearing clone URL', () => {
    const options = wizardRepositoryOptions({
      organizationId: ORG,
      connection,
      repositories: [
        repository({ cloneUrl: 'https://user:ghp_secret@github.com/acme/api.git' }),
        repository({
          id: REPO_ID_2,
          fullName: 'acme/web',
          cloneUrl: 'https://github.com/acme/web.git?token=ghp_query',
          metadata: { visibility: 'public' },
        }),
        repository({
          id: '550e8400-e29b-41d4-a716-446655440012',
          fullName: 'acme/safe',
          cloneUrl: 'https://github.com/acme/safe.git',
          metadata: { private: false },
        }),
      ],
    })
    assert.equal(options[0]?.selectable, false)
    assert.equal(options[0]?.selection, undefined)
    assert.equal(options[1]?.selectable, false)
    assert.equal(options[2]?.selectable, true)
    assert.equal(options[2]?.selection?.cloneUrl, 'https://github.com/acme/safe.git')
    const encoded = JSON.stringify(options)
    assert.equal(encoded.includes('ghp_secret'), false)
    assert.equal(encoded.includes('ghp_query'), false)
    assert.equal(encoded.includes('user:'), false)
    assert.equal(options.some((option) => option.label.includes('http')), false)
  })

  it('reports an empty PAT repository list without inventing repositories', () => {
    const options = wizardRepositoryOptions({
      organizationId: ORG,
      connection: patConnection(),
      repositories: [],
    })
    assert.deepEqual(options, [])
    assert.equal(
      repositoryListMessage({ connectionSelected: true, phase: 'ready', repositoryCount: 0 }),
      'No synchronized repositories',
    )
  })
})

describe('git source transitions', () => {
  const selected = form({
    gitConnectionId: APP_ID,
    repositoryId: REPO_ID,
    repository: SAFE_URL,
    branch: 'develop',
    dockerfile: 'Dockerfile.prod',
    buildContext: 'services/api',
  })

  it('clears the repository and URL when the connection changes', () => {
    const next = applyConnectionChange(selected, PAT_ID)
    assert.equal(next.gitConnectionId, PAT_ID)
    assert.equal(next.repositoryId, '')
    assert.equal(next.repository, '')
    assert.equal(next.branch, 'main')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'services/api')
    assert.equal(next.repositorySource, 'connected')
  })

  it('clears a private URL when switching from connected to public', () => {
    const next = applyPublicGitTransition(selected)
    assert.equal(next.repositorySource, 'public')
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.repository, '')
    assert.equal(next.branch, 'develop')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'services/api')
  })

  it('does not reuse a typed public URL when switching to connected', () => {
    const next = applyConnectedGitTransition(
      form({
        repositorySource: 'public',
        repository: 'https://github.com/acme/public.git',
        branch: 'feature',
        dockerfile: 'Dockerfile.prod',
        buildContext: 'app',
      }),
    )
    assert.equal(next.repositorySource, 'connected')
    assert.equal(next.repository, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.branch, 'feature')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'app')
  })

  it('clears connected state when Git changes to image or Compose', () => {
    for (const nextSource of ['docker-image', 'docker-compose']) {
      const next = applySourceTypeChange(selected, nextSource)
      assert.equal(next.sourceType, nextSource)
      assert.equal(next.gitConnectionId, '')
      assert.equal(next.repositoryId, '')
      assert.equal(next.repository, '')
      assert.equal(next.branch, 'develop')
      assert.equal(next.dockerfile, 'Dockerfile.prod')
      assert.equal(next.buildContext, 'services/api')
      const returned = applySourceTypeChange(next, 'git')
      assert.equal(returned.sourceType, 'git')
      assert.equal(returned.repository, '')
      assert.equal(returned.gitConnectionId, '')
      assert.equal(returned.repositoryId, '')
    }
  })

  it('clears Git selection and placement when the organization changes', () => {
    const next = applyOrganizationChange(selected)
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.repository, '')
    assert.equal(next.branch, 'main')
    assert.equal(next.projectId, '')
    assert.equal(next.environment, '')
    assert.equal(next.serverId, '')
    assert.equal(next.dockerfile, 'Dockerfile.prod')
    assert.equal(next.buildContext, 'services/api')
    assert.equal(next.repositorySource, 'connected')
  })

  it('clears a connection that is no longer selectable and stays in connected mode', () => {
    const options = wizardConnectionOptions([appConnection({ providerStatus: 'revoked' })], ORG)
    const next = connectionSelectionAfterRefresh(selected, options, ORG)
    assert.equal(next.repositorySource, 'connected')
    assert.equal(next.gitConnectionId, '')
    assert.equal(next.repositoryId, '')
    assert.equal(next.repository, '')
    assert.equal(next.branch, 'main')
  })
})

describe('stale repository loads', () => {
  const requested = scope()
  const repositories = [{ id: REPO_ID, fullName: 'acme/api' }]

  it('ignores a repository response after the connection, organization, mode, source, or wizard changes', () => {
    assert.equal(repositoryLoadResult(requested, scope({ organizationId: OTHER_ORG }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ connectionId: PAT_ID }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ repositorySource: 'public' }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ sourceType: 'docker-image' }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ sourceType: 'docker-compose' }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ open: false }), repositories), null)
    assert.equal(repositoryLoadResult(requested, scope({ generation: 2 }), repositories), null)
    assert.deepEqual(repositoryLoadResult(requested, scope(), repositories), repositories)
  })
})

describe('wizard page collection', () => {
  it('loads every connection page through the existing collector', async () => {
    const calls: { organizationId: string; limit: number; offset: number }[] = []
    const connections = await loadWizardGitConnections(ORG, async (page) => {
      calls.push(page)
      if (page.offset === 0) {
        return { items: Array.from({ length: 100 }, (_, index) => ({ id: `c-${index}` })), totalCount: 101 }
      }
      return { items: [{ id: 'c-100' }], totalCount: 101 }
    })
    assert.equal(connections.length, 101)
    assert.deepEqual(calls, [
      { organizationId: ORG, limit: REPOSITORY_PAGE_LIMIT, offset: 0 },
      { organizationId: ORG, limit: REPOSITORY_PAGE_LIMIT, offset: 100 },
    ])
  })

  it('loads repository pages above 100 through the existing collector', async () => {
    const calls: { limit: number; offset: number }[] = []
    const repositories = await loadWizardRepositories(async (page) => {
      calls.push(page)
      if (page.offset === 0) {
        return {
          items: Array.from({ length: 100 }, (_, index) => ({ id: `r-${index}` })),
          totalCount: 101,
        }
      }
      return { items: [{ id: 'r-100' }], totalCount: 101 }
    })
    assert.equal(repositories.length, 101)
    assert.equal(repositories[100]?.id, 'r-100')
    assert.deepEqual(calls, [
      { limit: 100, offset: 0 },
      { limit: 100, offset: 100 },
    ])
  })
})

describe('organization cache ownership', () => {
  const orgAConnections = wizardConnectionOptions([appConnection(), patConnection()], ORG)
  const orgARepositories = wizardRepositoryOptions({
    organizationId: ORG,
    connection: appConnection(),
    repositories: [repository()],
  })

  function loadedFor(organizationId: string, connections = orgAConnections): WizardGitCache {
    const loaded = wizardGitCacheAfterConnectionsLoaded(
      { organizationId: '', connections: [], repositories: [], connectionPhase: 'idle', repositoryPhase: 'idle' },
      { activeOrganizationId: organizationId, requestedOrganizationId: organizationId, connections },
    )
    assert.ok(loaded)
    return {
      ...loaded,
      repositories: organizationId === ORG ? orgARepositories : [],
      repositoryPhase: organizationId === ORG ? 'ready' : 'idle',
    }
  }

  it('drops organization A connections after the organization changes to B while the wizard is closed', () => {
    const opened = loadedFor(ORG)
    assert.equal(visibleWizardGitCache(opened, ORG).connections.length > 0, true)

    const closed = opened
    const switched = wizardGitCacheAfterOrganizationChange(closed, OTHER_ORG)
    const reopened = visibleWizardGitCache(switched, OTHER_ORG)

    assert.equal(reopened.organizationId, OTHER_ORG)
    const noConnections: WizardConnectionOption[] = []
    assert.deepEqual(reopened.connections, noConnections)
    assert.deepEqual(reopened.repositories, [])
    assert.equal(reopened.connections.some((connection) => connection.id === APP_ID), false)
    assert.equal(connectionSelectionAllowed(orgAConnections[0], OTHER_ORG), false)
    assert.equal(connectionSelectionAllowed(orgAConnections[1], OTHER_ORG), false)
    assert.equal(connectionSelectionAllowed(orgAConnections[0], ORG), true)
  })

  it('does not let a failed organization B reload preserve organization A connections', () => {
    const switched = wizardGitCacheAfterOrganizationChange(loadedFor(ORG), OTHER_ORG)
    const failed = wizardGitCacheAfterConnectionFailure(switched, {
      activeOrganizationId: OTHER_ORG,
      requestedOrganizationId: OTHER_ORG,
    })
    assert.ok(failed)
    assert.equal(failed.connectionPhase, 'error')
    assert.deepEqual(failed.connections, [])
    assert.deepEqual(visibleWizardGitCache(failed, OTHER_ORG).connections, [])

    const failedBeforeClear = wizardGitCacheAfterConnectionFailure(loadedFor(ORG), {
      activeOrganizationId: OTHER_ORG,
      requestedOrganizationId: OTHER_ORG,
    })
    assert.ok(failedBeforeClear)
    assert.equal(failedBeforeClear.organizationId, OTHER_ORG)
    assert.deepEqual(failedBeforeClear.connections, [])
    assert.deepEqual(failedBeforeClear.repositories, [])
  })

  it('keeps a same-organization cache and does not treat it as another tenant', () => {
    const opened = loadedFor(ORG)
    const unchanged = wizardGitCacheAfterOrganizationChange(opened, ORG)
    assert.equal(unchanged, opened)
    assert.equal(visibleWizardGitCache(unchanged, ORG).connections[0]?.id, APP_ID)
    assert.equal(visibleWizardGitCache(unchanged, OTHER_ORG).connections.length, 0)
  })

  it('does not apply an organization A repository response after the active organization is B', () => {
    const requested = scope()
    const orgAResponse = [{ id: REPO_ID, fullName: 'acme/api' }]
    assert.equal(repositoryLoadResult(requested, scope({ organizationId: OTHER_ORG }), orgAResponse), null)

    const retained = commitWizardGitCache(loadedFor(ORG), OTHER_ORG, null)
    assert.equal(retained.organizationId, OTHER_ORG)
    assert.deepEqual(retained.connections, [])
    assert.deepEqual(retained.repositories, [])

    const ignored = wizardGitCacheAfterRepositoriesLoaded(loadedFor(ORG), {
      activeOrganizationId: OTHER_ORG,
      requestedOrganizationId: ORG,
      repositories: orgARepositories,
      phase: 'ready',
    })
    assert.equal(ignored, null)
    assert.deepEqual(visibleWizardGitCache(loadedFor(ORG), OTHER_ORG).repositories, [])

    const cleared = wizardGitCacheAfterRepositoriesLoaded(loadedFor(ORG), {
      activeOrganizationId: OTHER_ORG,
      requestedOrganizationId: OTHER_ORG,
      repositories: orgARepositories,
      phase: 'ready',
    })
    assert.ok(cleared)
    assert.equal(cleared.organizationId, OTHER_ORG)
    assert.deepEqual(cleared.connections, [])
    assert.deepEqual(cleared.repositories, [])
  })
})

describe('empty and error copy', () => {
  it('uses the connection and repository status messages', () => {
    assert.equal(connectionListMessage({ phase: 'loading', visibleCount: 0, selectableCount: 0 }), 'Loading Git connections…')
    assert.equal(connectionListMessage({ phase: 'error', visibleCount: 0, selectableCount: 0 }), 'Unable to load Git connections')
    assert.equal(connectionListMessage({ phase: 'ready', visibleCount: 1, selectableCount: 0 }), 'No active Git connections')
    assert.equal(repositoryListMessage({ connectionSelected: false, phase: 'ready', repositoryCount: 0 }), 'Select a connection first')
    assert.equal(repositoryListMessage({ connectionSelected: true, phase: 'loading', repositoryCount: 0 }), 'Loading repositories…')
    assert.equal(repositoryListMessage({ connectionSelected: true, phase: 'error', repositoryCount: 0 }), 'Unable to load repositories')
  })
})
