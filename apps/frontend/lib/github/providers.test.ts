import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { mapWireGitConnection, type WireGitConnection } from './git-connection'
import {
  accessForOrganizationUser,
  applySyncResult,
  authModeLabel,
  beginGitHubInstallationRequest,
  collectRepositoryPages,
  connectionStatusView,
  countsForOrganization,
  disconnectCopy,
  gitAccessFromRoles,
  gitConnectionSyncRequest,
  gitHubAppStatusKind,
  githubManageUrl,
  installationNavigationTarget,
  isGitHubAppConnection,
  repositoriesToDisplayAfterSync,
  repositorySelectionLabel,
  repositoryTotalCount,
  safeRepositoryHref,
  shouldBeginInstallation,
} from './providers'

function wire(overrides: Partial<WireGitConnection> = {}): WireGitConnection {
  return {
    id: '33333333-3333-4333-8333-333333333333',
    organizationId: '22222222-2222-4222-8222-222222222222',
    provider: 'github',
    accountLogin: 'octo',
    displayName: 'Octo',
    status: 'active',
    authMode: 'github_app',
    hasWebhookSecret: false,
    createdAt: '2026-10-04T00:00:00Z',
    updatedAt: '2026-10-04T00:00:00Z',
    ...overrides,
  }
}

describe('GitHub App status', () => {
  it('offers Connect GitHub only when the app is configured', () => {
    assert.equal(gitHubAppStatusKind({ loading: false, error: null, configured: true }), 'configured')
    assert.equal(
      shouldBeginInstallation({
        pending: false,
        organizationId: 'org-1',
        configured: true,
        canManage: true,
      }),
      true,
    )
  })

  it('hides Connect GitHub when the app is not configured and keeps token access possible', () => {
    assert.equal(gitHubAppStatusKind({ loading: false, error: null, configured: false }), 'unconfigured')
    assert.equal(
      shouldBeginInstallation({
        pending: false,
        organizationId: 'org-1',
        configured: false,
        canManage: true,
      }),
      false,
    )
  })

  it('does not treat a status failure as unconfigured', () => {
    assert.equal(
      gitHubAppStatusKind({ loading: false, error: 'network down', configured: null }),
      'unavailable',
    )
    assert.notEqual(
      gitHubAppStatusKind({ loading: false, error: 'network down', configured: null }),
      'unconfigured',
    )
  })
})

describe('Connect GitHub request', () => {
  it('sends only the current organization id', () => {
    assert.deepEqual(beginGitHubInstallationRequest('org-a'), {
      path: '/integrations/github/installations',
      body: { organizationId: 'org-a' },
    })
    assert.deepEqual(Object.keys(beginGitHubInstallationRequest('org-b').body), ['organizationId'])
  })

  it('uses the returned installation URL unchanged', () => {
    const url = 'https://github.com/apps/deploycore/installations/new?state=opaque'
    assert.equal(installationNavigationTarget(url), url)
    assert.equal(installationNavigationTarget('javascript:alert(1)'), null)
  })

  it('ignores a second click while a begin request is pending', () => {
    const first = shouldBeginInstallation({
      pending: false,
      organizationId: 'org-1',
      configured: true,
      canManage: true,
    })
    const second = shouldBeginInstallation({
      pending: true,
      organizationId: 'org-1',
      configured: true,
      canManage: true,
    })
    assert.equal(first, true)
    assert.equal(second, false)
  })

  it('does not begin an installation for a viewer', () => {
    const access = gitAccessFromRoles([{ key: 'viewer', name: 'Viewer' }])
    assert.equal(access.canRead, true)
    assert.equal(access.canManage, false)
    assert.equal(
      shouldBeginInstallation({
        pending: false,
        organizationId: 'org-1',
        configured: true,
        canManage: access.canManage,
      }),
      false,
    )
  })
})

describe('connection presentation', () => {
  it('labels GitHub App and PAT auth modes without treating a fixture as an app', () => {
    assert.equal(authModeLabel('github_app'), 'GitHub App')
    assert.equal(authModeLabel('pat'), 'Personal access token')
    assert.equal(authModeLabel(undefined), null)
    assert.equal(isGitHubAppConnection({}), false)
    assert.equal(isGitHubAppConnection({ authMode: 'pat' }), false)
  })

  it('maps disabled to Suspended and revoked to Revoked', () => {
    assert.equal(connectionStatusView('disabled').label, 'Suspended')
    assert.notEqual(connectionStatusView('disabled').label, 'Revoked')
    assert.equal(connectionStatusView('revoked').label, 'Revoked')
    assert.equal(connectionStatusView('active').label, 'Active')
    assert.equal(connectionStatusView('error').label, 'Error')
  })

  it('labels repository selection', () => {
    assert.equal(repositorySelectionLabel('all'), 'All repositories')
    assert.equal(repositorySelectionLabel('selected'), 'Selected repositories')
  })

  it('does not use metadata.repositoryCount for a GitHub App', () => {
    const mapped = mapWireGitConnection(
      wire({ metadata: { repositoryCount: 99 } }),
    )
    assert.equal(mapped.repositoryCountKnown, false)
    assert.notEqual(mapped.repositoryCount, 99)
  })

  it('uses repository totalCount when it is present', () => {
    assert.equal(repositoryTotalCount({ totalCount: 120, items: [{}] } as { totalCount: number }), 120)
    assert.equal(repositoryTotalCount({ totalCount: 0 }), 0)
    assert.equal(repositoryTotalCount({}), null)
    const mapped = mapWireGitConnection(wire({ metadata: { repositoryCount: 99 } }), 120)
    assert.equal(mapped.repositoryCount, 120)
    assert.equal(mapped.repositoryCountKnown, true)
  })
})

describe('repository sync', () => {
  it('posts an empty body', () => {
    const request = gitConnectionSyncRequest('connection-1')
    assert.equal(request.path, '/integrations/git/connections/connection-1/sync')
    assert.deepEqual(request.body, {})
  })

  it('updates a GitHub App connection from the sync response instead of metadata', () => {
    const updated = applySyncResult({
      connection: wire({
        status: 'active',
        lastSyncAt: '2026-10-04T01:00:00Z',
        metadata: { repositoryCount: 99 },
      }),
      repositories: [{ id: 'a' }, { id: 'b' }, { id: 'c' }] as never,
    })
    assert.equal(updated.authMode, 'github_app')
    assert.equal(updated.repositoryCount, 3)
    assert.equal(updated.repositoryCountKnown, true)
    assert.equal(updated.providerStatus, 'active')
    assert.notEqual(updated.lastSync, 'Never')
  })

  it('keeps a PAT connection on the token path', () => {
    const updated = applySyncResult({
      connection: wire({
        authMode: 'pat',
        metadata: { repositoryCount: 4, permissions: ['read:repo'] },
      }),
      repositories: [{ id: 'a' }, { id: 'b' }] as never,
    })
    assert.equal(updated.authMode, 'pat')
    assert.equal(authModeLabel(updated.authMode), 'Personal access token')
    assert.equal(updated.repositoryCount, 2)
    assert.equal(isGitHubAppConnection(updated), false)
  })

  it('reloads stored repositories after a PAT sync that returns an empty list', async () => {
    const stored = [{ id: 'repo-1', fullName: 'acme/kept' }]
    const syncRepositories: unknown[] = []
    const displayed = await repositoriesToDisplayAfterSync({
      previous: stored,
      syncRepositories,
      loadPages: async (page) => {
        assert.equal(page.limit, 100)
        assert.equal(page.offset, 0)
        return { items: stored, totalCount: stored.length }
      },
    })
    assert.equal(displayed.preservedPrevious, false)
    assert.deepEqual(displayed.repositories, stored)
    assert.notEqual(displayed.repositories, syncRepositories)
  })

  it('keeps the previous dialog list when the reload after sync fails', async () => {
    const stored = [{ id: 'repo-1', fullName: 'acme/kept' }]
    const displayed = await repositoriesToDisplayAfterSync({
      previous: stored,
      syncRepositories: [],
      loadPages: async () => {
        throw new Error('list failed')
      },
    })
    assert.equal(displayed.preservedPrevious, true)
    assert.equal(displayed.repositories, stored)
    assert.equal(displayed.reloadError instanceof Error, true)
  })
})

describe('Manage on GitHub', () => {
  it('builds a user installation URL', () => {
    assert.equal(
      githubManageUrl({
        authMode: 'github_app',
        installationId: 42,
        accountType: 'User',
        accountLogin: 'octo',
      }),
      'https://github.com/settings/installations/42',
    )
  })

  it('builds an organization installation URL', () => {
    assert.equal(
      githubManageUrl({
        authMode: 'github_app',
        installationId: 42,
        accountType: 'Organization',
        accountLogin: 'acme',
      }),
      'https://github.com/organizations/acme/settings/installations/42',
    )
  })

  it('hides the action when metadata is insufficient or the connection is a token', () => {
    assert.equal(
      githubManageUrl({ authMode: 'github_app', installationId: 42, accountType: 'Organization' }),
      null,
    )
    assert.equal(
      githubManageUrl({ authMode: 'github_app', accountType: 'User', accountLogin: 'octo' }),
      null,
    )
    assert.equal(
      githubManageUrl({
        authMode: 'pat',
        installationId: 42,
        accountType: 'User',
        accountLogin: 'octo',
      }),
      null,
    )
  })
})

describe('repository pages and organization scope', () => {
  it('loads pages past the first 100 repositories', async () => {
    const calls: Array<{ limit: number; offset: number }> = []
    const items = await collectRepositoryPages(async (page) => {
      calls.push(page)
      const start = page.offset
      const slice = Array.from({ length: Math.min(page.limit, Math.max(0, 250 - start)) }, (_, index) => start + index)
      return { items: slice, totalCount: 250 }
    })
    assert.equal(items.length, 250)
    assert.deepEqual(calls, [
      { limit: 100, offset: 0 },
      { limit: 100, offset: 100 },
      { limit: 100, offset: 200 },
    ])
  })

  it('drops counts that belong to another organization', () => {
    const snapshot = { organizationId: 'org-a', counts: { connection: 4 } }
    assert.deepEqual(countsForOrganization('org-a', snapshot), { connection: 4 })
    assert.equal(countsForOrganization('org-b', snapshot), null)
  })

  it('rejects a credential-bearing repository URL', () => {
    assert.equal(safeRepositoryHref('https://github.com/acme/app'), 'https://github.com/acme/app')
    assert.equal(safeRepositoryHref('https://x-access-token:secret@github.com/acme/app.git'), null)
  })
})

describe('disconnect copy', () => {
  it('does not claim the GitHub App is uninstalled', () => {
    const copy = disconnectCopy({ authMode: 'github_app', account: 'octo', type: 'GitHub' })
    assert.match(copy.description, /DeployCore will disconnect/)
    assert.match(copy.description, /does not uninstall the GitHub App from GitHub/)
    assert.doesNotMatch(copy.description, /will uninstall|uninstalled the GitHub App/i)
  })
})

describe('organization member access', () => {
  it('finds a later page and does not grant manage to a viewer', async () => {
    const access = await accessForOrganizationUser('user-2', async (offset) => {
      if (offset === 0) {
        return {
          items: Array.from({ length: 100 }, (_, index) => ({
            userId: `other-${index}`,
            roles: [{ key: 'owner', name: 'Owner' }],
          })),
          totalCount: 101,
        }
      }
      return {
        items: [{ userId: 'user-2', roles: [{ key: 'viewer', name: 'Viewer' }] }],
        totalCount: 101,
      }
    })
    assert.equal(access.known, true)
    assert.equal(access.canRead, true)
    assert.equal(access.canManage, false)
  })
})
