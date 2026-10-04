import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { mapWireGitConnection, type WireGitConnection } from './git-connection'

function base(overrides: Partial<WireGitConnection> = {}): WireGitConnection {
  return {
    id: '33333333-3333-4333-8333-333333333333',
    organizationId: '22222222-2222-4222-8222-222222222222',
    provider: 'github',
    accountLogin: 'octo',
    displayName: 'Octo',
    status: 'active',
    hasWebhookSecret: false,
    createdAt: '2026-10-04T00:00:00Z',
    updatedAt: '2026-10-04T00:00:00Z',
    ...overrides,
  }
}

describe('mapWireGitConnection', () => {
  it('keeps PAT rows rendering with the previous permission fallback', () => {
    const mapped = mapWireGitConnection(
      base({
        authMode: 'pat',
        metadata: { repositoryCount: 4, organizations: ['acme'] },
      }),
    )
    assert.equal(mapped.authMode, 'pat')
    assert.equal(mapped.repositoryCount, 4)
    assert.equal(mapped.repositoryCountKnown, true)
    assert.deepEqual(mapped.permissions, ['read:repo', 'read:org'])
    assert.deepEqual(mapped.organizations, ['acme'])
    assert.equal(mapped.accountType, '')
    assert.equal(mapped.repositorySelection, '')
    assert.equal(mapped.installationId, null)
    assert.equal(mapped.status, 'healthy')
  })

  it('keeps GitHub App identity and does not invent permissions or a repository count', () => {
    const mapped = mapWireGitConnection(
      base({
        authMode: 'github_app',
        installationId: 42,
        accountId: '99',
        accountType: 'Organization',
        repositorySelection: 'selected',
        status: 'disabled',
        metadata: { repositoryCount: 99, permissions: ['admin'] },
      }),
    )
    assert.equal(mapped.authMode, 'github_app')
    assert.equal(mapped.installationId, 42)
    assert.equal(mapped.accountId, '99')
    assert.equal(mapped.accountType, 'Organization')
    assert.equal(mapped.repositorySelection, 'selected')
    assert.equal(mapped.repositoryCount, 0)
    assert.equal(mapped.repositoryCountKnown, false)
    assert.deepEqual(mapped.permissions, [])
    assert.equal(mapped.providerStatus, 'disabled')
    assert.equal('webhookSecret' in mapped, false)
  })

  it('uses an explicit repository count for a GitHub App connection', () => {
    const mapped = mapWireGitConnection(
      base({ authMode: 'github_app', metadata: { repositoryCount: 99 } }),
      3,
    )
    assert.equal(mapped.repositoryCount, 3)
    assert.equal(mapped.repositoryCountKnown, true)
  })
})
