import assert from 'node:assert/strict'
import { beforeEach, describe, it } from 'node:test'

import type { CompleteGitHubInstallationRequest, WireGitConnection } from './git-connection'
import {
  beginGitHubCallback,
  canonicalCallbackIdentity,
  pauseGitHubCallbackLifecycle,
  resetGitHubCallbackLifecycleForTests,
} from './callback-lifecycle'

const QUERY_A = 'installation_id=10&setup_action=install&state=A'
const QUERY_A_REORDERED = 'state=A&setup_action=install&installation_id=10'
const QUERY_B = 'installation_id=11&setup_action=install&state=B'

function connection(): WireGitConnection {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    organizationId: '22222222-2222-4222-8222-222222222222',
    provider: 'github',
    accountLogin: 'octo',
    displayName: 'octo',
    status: 'active',
    authMode: 'github_app',
    hasWebhookSecret: false,
    createdAt: '2026-10-04T00:00:00Z',
    updatedAt: '2026-10-04T00:00:00Z',
  }
}

function success(state: string) {
  return { connection: connection(), repositoryCount: state === 'A' ? 1 : 2 }
}

async function flushLifecycleRelease(): Promise<void> {
  await new Promise<void>((resolve) => {
    queueMicrotask(resolve)
  })
}

describe('GitHub callback lifecycle', () => {
  beforeEach(() => {
    resetGitHubCallbackLifecycleForTests()
  })

  it('submits the same callback once across an immediate remount', async () => {
    let calls = 0
    const complete = async (body: CompleteGitHubInstallationRequest) => {
      calls += 1
      return success(body.state)
    }

    const first = beginGitHubCallback(QUERY_A, complete)
    pauseGitHubCallbackLifecycle()
    const remount = beginGitHubCallback('', complete)

    assert.equal(first, remount)
    assert.equal((await first).kind, 'SUCCESS')
    assert.equal(calls, 1)
  })

  it('treats reordered parameters as the same callback during the lifecycle', async () => {
    assert.equal(canonicalCallbackIdentity(QUERY_A), canonicalCallbackIdentity(QUERY_A_REORDERED))

    let calls = 0
    const complete = async () => {
      calls += 1
      return success('A')
    }

    const first = beginGitHubCallback(QUERY_A, complete)
    const reordered = beginGitHubCallback(QUERY_A_REORDERED, complete)
    pauseGitHubCallbackLifecycle()
    const afterCleanup = beginGitHubCallback('', complete)

    assert.equal(first, reordered)
    assert.equal(reordered, afterCleanup)
    await first
    assert.equal(calls, 1)
  })

  it('completes a different callback independently', async () => {
    const seen: string[] = []
    const complete = async (body: CompleteGitHubInstallationRequest) => {
      seen.push(body.state)
      return success(body.state)
    }

    await beginGitHubCallback(QUERY_A, complete)
    const second = await beginGitHubCallback(QUERY_B, complete)

    assert.deepEqual(seen, ['A', 'B'])
    assert.equal(second.kind, 'SUCCESS')
    if (second.kind === 'SUCCESS') assert.equal(second.repositoryCount, 2)
  })

  it('completes callback B after callback A fails', async () => {
    const seen: string[] = []
    const complete = async (body: CompleteGitHubInstallationRequest) => {
      seen.push(body.state)
      if (body.state === 'A') {
        throw { status: 502, message: 'GitHub API request failed' }
      }
      return success(body.state)
    }

    const failed = await beginGitHubCallback(QUERY_A, complete)
    const next = await beginGitHubCallback(QUERY_B, complete)

    assert.equal(failed.kind, 'GITHUB_UNAVAILABLE')
    assert.equal(next.kind, 'SUCCESS')
    assert.deepEqual(seen, ['A', 'B'])
  })

  it('does not replay a finished callback on a later bare visit', async () => {
    let calls = 0
    const complete = async () => {
      calls += 1
      return success('A')
    }

    const finished = await beginGitHubCallback(QUERY_A, complete)
    pauseGitHubCallbackLifecycle()
    await flushLifecycleRelease()

    const bare = await beginGitHubCallback('', complete)

    assert.equal(finished.kind, 'SUCCESS')
    assert.equal(bare.kind, 'NOT_FINISHED')
    assert.notEqual(finished, bare)
    assert.equal(calls, 1)
  })

  it('runs a later callback after the previous lifecycle has ended', async () => {
    const seen: string[] = []
    const complete = async (body: CompleteGitHubInstallationRequest) => {
      seen.push(body.state)
      return success(body.state)
    }

    await beginGitHubCallback(QUERY_A, complete)
    pauseGitHubCallbackLifecycle()
    await flushLifecycleRelease()
    const next = await beginGitHubCallback(QUERY_B, complete)

    assert.deepEqual(seen, ['A', 'B'])
    assert.equal(next.kind, 'SUCCESS')
  })

  it('does not submit twice when the effect re-runs after the query is removed', async () => {
    let calls = 0
    const complete = async () => {
      calls += 1
      return success('A')
    }

    const first = beginGitHubCallback(`?${QUERY_A}`, complete)
    pauseGitHubCallbackLifecycle()
    const rerun = beginGitHubCallback('', complete)

    assert.equal(await first, await rerun)
    assert.equal(calls, 1)
  })
})
