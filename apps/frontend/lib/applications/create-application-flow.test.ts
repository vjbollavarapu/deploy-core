import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  applicationVariableBodies,
  CreateFlowError,
  createApplicationWithVariables,
  creationMode,
  isPersistedVariableKey,
  redactVariableValues,
  type FlowClient,
} from './create-application-flow'

type Call = { path: string; body: unknown }

function client(handlers: Record<string, (body: unknown, calls: Call[]) => unknown>): {
  calls: Call[]
  client: FlowClient
} {
  const calls: Call[] = []
  return {
    calls,
    client: {
      async post<T>(path: string, body?: unknown): Promise<T> {
        calls.push({ path, body })
        const handler = handlers[path] ?? handlers['*']
        if (!handler) return {} as T
        return handler(body, calls) as T
      },
    },
  }
}

describe('create application variable order', () => {
  it('posts every variable before deployment', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': () => ({ variable: { id: 'v' } }),
      '/applications/app-1/deployments': () => ({ deployment: { id: 'd-1' } }),
    })

    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'redis' },
      envVars: [
        { key: 'REDIS_URL', value: 'redis://redis:6379' },
        { key: 'LOG_LEVEL', value: 'info' },
      ],
    })

    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/applications', '/variables', '/variables', '/applications/app-1/deployments'],
    )
    assert.deepEqual(harness.calls[1].body, {
      organizationId: 'org-1',
      scope: 'APPLICATION',
      applicationId: 'app-1',
      key: 'REDIS_URL',
      value: 'redis://redis:6379',
    })
    assert.equal((harness.calls[2].body as { key: string }).key, 'LOG_LEVEL')
    assert.deepEqual(harness.calls[3].body, { trigger: 'manual' })
  })

  it('queues deployment when no variables were entered', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/applications/app-1/deployments': () => ({}),
    })
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [],
    })
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/applications', '/applications/app-1/deployments'],
    )
  })

  it('does not queue a deployment when a variable write fails', async () => {
    const secret = 'redis://redis:6379'
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': (body) => {
        const key = (body as { key: string }).key
        if (key === 'BROKEN') {
          throw new Error(`invalid variable value ${secret}`)
        }
        return {}
      },
    })

    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          applicationBody: { name: 'api' },
          envVars: [
            { key: 'OK', value: 'yes' },
            { key: 'BROKEN', value: secret },
          ],
        }),
      (err: Error) => {
        assert.match(err.message, /could not be saved/)
        assert.match(err.message, /Deployment was not queued/)
        assert.equal(err.message.includes(secret), false)
        assert.match(err.message, /\[REDACTED\]/)
        return true
      },
    )
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/applications', '/variables', '/variables'],
    )
  })

  it('keeps saved variables when deployment creation fails', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': () => ({}),
      '/applications/app-1/deployments': () => {
        throw new Error('deployment queue unavailable')
      },
    })
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          applicationBody: { name: 'api' },
          envVars: [{ key: 'LOG_LEVEL', value: 'info' }],
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.equal(err.phase, 'deployment')
        assert.deepEqual(err.savedKeys, ['LOG_LEVEL'])
        assert.match(err.message, /environment variables were saved/)
        assert.equal(err.message.includes('info'), false)
        return true
      },
    )
    assert.equal(
      harness.calls.some((call) => call.path.includes('DELETE')),
      false,
    )
    assert.equal(harness.calls.filter((call) => call.path === '/variables').length, 1)
  })

  it('does not place secret names on the variable endpoint', () => {
    const bodies = applicationVariableBodies('org-1', 'app-1', [
      { key: 'LOG_LEVEL', value: 'info' },
    ])
    assert.equal(JSON.stringify(bodies).includes('SECRET'), false)
    assert.equal(bodies[0].scope, 'APPLICATION')
  })

  it('retries only the deployment after variables were saved', async () => {
    const harness = client({
      '/applications/app-1/deployments': () => ({ deployment: { id: 'd-1' } }),
    })
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [{ key: 'LOG_LEVEL', value: 'info' }],
      resume: { applicationId: 'app-1', savedKeys: ['LOG_LEVEL'] },
    })
    assert.deepEqual(harness.calls.map((call) => call.path), ['/applications/app-1/deployments'])
  })

  it('resumes unsaved variables without recreating the application or reposting saved keys', async () => {
    const secret = 'b-secret-value'
    let attempts = 0
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': (body) => {
        const key = (body as { key: string }).key
        if (key === 'B') {
          attempts += 1
          if (attempts < 3) throw new Error(`rejected ${secret}`)
        }
        return {}
      },
      '/applications/app-1/deployments': () => ({ deployment: { id: 'd-1' } }),
    })
    const envVars = [
      { key: 'A', value: 'a-value' },
      { key: 'B', value: secret },
      { key: 'C', value: 'c-value' },
    ]

    let savedKeys: string[] = []
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          applicationBody: { name: 'api' },
          envVars,
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.equal(err.phase, 'configuration')
        assert.equal(err.applicationId, 'app-1')
        assert.deepEqual(err.savedKeys, ['A'])
        assert.match(err.message, /Application already exists/)
        assert.match(err.message, /Some configuration was saved/)
        assert.match(err.message, /Deployment was not queued/)
        assert.match(err.message, /Press Deploy again/)
        assert.equal(err.message.includes(secret), false)
        assert.equal(err.message.includes('a-value'), false)
        assert.equal(err.message.includes('c-value'), false)
        savedKeys = err.savedKeys
        return true
      },
    )
    assert.deepEqual(
      harness.calls.map((call) => [call.path, (call.body as { key?: string } | undefined)?.key]),
      [
        ['/applications', undefined],
        ['/variables', 'A'],
        ['/variables', 'B'],
      ],
    )

    harness.calls.length = 0
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          applicationBody: { name: 'api' },
          envVars,
          resume: { applicationId: 'app-1', savedKeys },
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.deepEqual(err.savedKeys, ['A'])
        assert.equal(err.message.includes(secret), false)
        savedKeys = err.savedKeys
        return true
      },
    )
    assert.deepEqual(
      harness.calls.map((call) => [call.path, (call.body as { key?: string } | undefined)?.key]),
      [['/variables', 'B']],
    )

    harness.calls.length = 0
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars,
      resume: { applicationId: 'app-1', savedKeys },
    })
    assert.deepEqual(
      harness.calls.map((call) => [call.path, (call.body as { key?: string } | undefined)?.key]),
      [
        ['/variables', 'B'],
        ['/variables', 'C'],
        ['/applications/app-1/deployments', undefined],
      ],
    )
  })

  it('does not post or deploy when a saved variable is removed', async () => {
    const harness = client({
      '/variables': () => ({}),
      '/applications/app-1/deployments': () => ({}),
    })
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          applicationBody: { name: 'api' },
          envVars: [{ key: 'C', value: 'c-value' }],
          resume: { applicationId: 'app-1', savedKeys: ['A'] },
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.match(err.message, /was not changed/)
        assert.match(err.message, /Deployment was not queued/)
        assert.equal(err.message.includes('c-value'), false)
        assert.deepEqual(err.savedKeys, ['A'])
        return true
      },
    )
    assert.equal(harness.calls.length, 0)
    assert.equal(isPersistedVariableKey(['A'], ' A '), true)
    assert.equal(isPersistedVariableKey(['A'], 'B'), false)
  })

  it('posts only a newly added variable after earlier keys were saved', async () => {
    const harness = client({
      '/variables': () => ({}),
      '/applications/app-1/deployments': () => ({}),
    })
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [
        { key: 'A', value: 'a-value' },
        { key: 'C', value: 'c-value' },
      ],
      resume: { applicationId: 'app-1', savedKeys: ['A'] },
    })
    assert.deepEqual(
      harness.calls.map((call) => [call.path, (call.body as { key?: string } | undefined)?.key]),
      [
        ['/variables', 'C'],
        ['/applications/app-1/deployments', undefined],
      ],
    )
  })

  it('redacts values and keeps demo mode off the production path', () => {
    assert.equal(redactVariableValues('failed for s3cret-value', ['s3cret-value']).includes('s3cret-value'), false)
    assert.equal(creationMode(true, false), 'api')
    assert.equal(creationMode(false, true), 'demo')
    assert.equal(creationMode(false, false), 'blocked')
  })
})
