import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  mapGitHubCallbackError,
  mapGitHubCompleteSuccess,
  planGitHubCallback,
  sanitizeAnalyticsUrl,
  settleGitHubCallback,
} from './callback'
import type { CompleteGitHubInstallationResponse, WireGitConnection } from './git-connection'

const CONNECTION_ID = '11111111-1111-4111-8111-111111111111'

function connection(overrides: Partial<WireGitConnection> = {}): WireGitConnection {
  return {
    id: CONNECTION_ID,
    organizationId: '22222222-2222-4222-8222-222222222222',
    provider: 'github',
    accountLogin: 'octo',
    displayName: 'octo',
    status: 'active',
    authMode: 'github_app',
    installationId: 42,
    accountId: '9',
    accountType: 'Organization',
    repositorySelection: 'all',
    hasWebhookSecret: false,
    createdAt: '2026-10-04T00:00:00Z',
    updatedAt: '2026-10-04T00:00:00Z',
    ...overrides,
  }
}

describe('GitHub callback query', () => {
  it('plans a valid install callback', () => {
    const plan = planGitHubCallback('installation_id=42&setup_action=install&state=abc')
    assert.deepEqual(plan, {
      action: 'complete',
      body: { installationId: 42, setupAction: 'install', state: 'abc' },
    })
    assert.equal('organizationId' in (plan.action === 'complete' ? plan.body : {}), false)
  })

  it('plans a valid update callback', () => {
    const plan = planGitHubCallback('?installation_id=7&setup_action=update&state=opaque')
    assert.deepEqual(plan, {
      action: 'complete',
      body: { installationId: 7, setupAction: 'update', state: 'opaque' },
    })
  })

  it('maps setup_action=request to NOT_FINISHED without a complete request', async () => {
    const plan = planGitHubCallback('installation_id=42&setup_action=request&state=abc')
    assert.deepEqual(plan, { action: 'local', outcome: { kind: 'NOT_FINISHED' } })
    let calls = 0
    const outcome = await settleGitHubCallback('installation_id=42&setup_action=request&state=abc', async () => {
      calls += 1
      return { connection: connection(), repositoryCount: 0 }
    })
    assert.equal(calls, 0)
    assert.equal(outcome.kind, 'NOT_FINISHED')
  })

  it('maps a missing installation id to NOT_FINISHED', () => {
    const plan = planGitHubCallback('setup_action=install&state=abc')
    assert.deepEqual(plan, { action: 'local', outcome: { kind: 'NOT_FINISHED' } })
  })

  it('maps a missing state to INVALID_CALLBACK', () => {
    const plan = planGitHubCallback('installation_id=42&setup_action=install')
    assert.deepEqual(plan, { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } })
  })

  it('rejects zero, negative, and non-numeric installation ids', () => {
    for (const installationId of ['0', '-1', 'abc', '1.5', '01']) {
      const plan = planGitHubCallback(`installation_id=${installationId}&setup_action=install&state=abc`)
      assert.deepEqual(plan, { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } })
    }
  })

  it('rejects an unknown setup action', () => {
    const plan = planGitHubCallback('installation_id=42&setup_action=delete&state=abc')
    assert.deepEqual(plan, { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } })
  })
})

describe('GitHub callback outcomes', () => {
  it('maps a success response', () => {
    const body: CompleteGitHubInstallationResponse = { connection: connection(), repositoryCount: 2 }
    assert.deepEqual(mapGitHubCompleteSuccess(body), {
      kind: 'SUCCESS',
      connection: body.connection,
      repositoryCount: 2,
    })
  })

  it('maps an expired or invalid state', () => {
    assert.equal(
      mapGitHubCallbackError({ status: 400, code: 'VALIDATION_ERROR', message: 'installation state has expired' }).kind,
      'EXPIRED_OR_INVALID',
    )
    assert.equal(
      mapGitHubCallbackError({ status: 400, message: 'installation state is invalid' }).kind,
      'EXPIRED_OR_INVALID',
    )
  })

  it('maps a replayed state', () => {
    assert.equal(
      mapGitHubCallbackError({ status: 409, code: 'CONFLICT', message: 'installation state has already been used' }).kind,
      'ALREADY_USED',
    )
  })

  it('maps an installation connected elsewhere', () => {
    assert.equal(
      mapGitHubCallbackError({
        status: 409,
        message: 'GitHub installation is already connected to another organization',
      }).kind,
      'ALREADY_CONNECTED_ELSEWHERE',
    )
  })

  it('maps a suspended installation', () => {
    assert.deepEqual(
      mapGitHubCallbackError({
        status: 409,
        message: 'GitHub installation is suspended',
        details: { connectionId: CONNECTION_ID, status: 'disabled' },
      }),
      { kind: 'SUSPENDED', connectionId: CONNECTION_ID },
    )
  })

  it('maps a sync failure that includes a connection id', () => {
    assert.deepEqual(
      mapGitHubCallbackError({
        status: 502,
        code: 'SERVICE_UNAVAILABLE',
        message: 'GitHub repository sync failed',
        details: { connectionId: CONNECTION_ID, status: 'error' },
      }),
      { kind: 'SYNC_FAILED', connectionId: CONNECTION_ID },
    )
    assert.equal(
      mapGitHubCallbackError({
        status: 503,
        message: 'GitHub repository sync failed',
        details: { connectionId: CONNECTION_ID, status: 'error' },
      }).kind,
      'SYNC_FAILED',
    )
  })

  it('maps permission denied', () => {
    assert.equal(
      mapGitHubCallbackError({ status: 403, code: 'FORBIDDEN', message: 'installation state belongs to another user' }).kind,
      'PERMISSION_DENIED',
    )
  })

  it('maps an unconfigured app', () => {
    assert.equal(
      mapGitHubCallbackError({ status: 503, message: 'GitHub App is not configured' }).kind,
      'APP_NOT_CONFIGURED',
    )
  })

  it('maps GitHub 404, 429, and 502 without a saved connection', () => {
    assert.equal(mapGitHubCallbackError({ status: 404, message: 'GitHub installation not found' }).kind, 'GITHUB_UNAVAILABLE')
    assert.equal(mapGitHubCallbackError({ status: 429, code: 'RATE_LIMITED', message: 'GitHub rate limit reached' }).kind, 'GITHUB_UNAVAILABLE')
    assert.equal(mapGitHubCallbackError({ status: 502, message: 'GitHub API request failed' }).kind, 'GITHUB_UNAVAILABLE')
  })

  it('maps an unknown error', () => {
    assert.equal(mapGitHubCallbackError(new Error('boom')).kind, 'UNKNOWN_ERROR')
    assert.equal(mapGitHubCallbackError({ status: 500, message: 'internal server error' }).kind, 'UNKNOWN_ERROR')
  })
})

describe('analytics URL sanitizing', () => {
  it('removes the callback query before an event is sent', () => {
    const sanitized = sanitizeAnalyticsUrl(
      'https://deploycore.example/integrations/git/github/callback?installation_id=42&setup_action=install&state=abc',
    )
    assert.equal(sanitized, 'https://deploycore.example/integrations/git/github/callback')
    assert.equal(sanitized.includes('state'), false)
  })

  it('removes state nested in a login next path', () => {
    const next = encodeURIComponent(
      '/integrations/git/github/callback?installation_id=42&setup_action=install&state=abc',
    )
    const sanitized = sanitizeAnalyticsUrl(`https://deploycore.example/login?next=${next}`)
    assert.equal(sanitized.includes('state'), false)
    assert.equal(sanitized.includes('installation_id'), true)
  })
})
