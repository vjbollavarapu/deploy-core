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

function client(
  handlers: Record<string, (body: unknown, calls: Call[]) => unknown>,
  gets: Record<string, (calls: Call[]) => unknown> = {},
): {
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
      async get<T>(path: string): Promise<T> {
        calls.push({ path, body: undefined })
        const handler = gets[path] ?? gets['*']
        if (!handler) return {} as T
        return handler(calls) as T
      },
    },
  }
}

describe('create application variable order', () => {
  it('saves the application and variables and returns the id without deploying', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': () => ({ variable: { id: 'v' } }),
      '/applications/app-1/deployments': () => {
        throw new Error('deployment must not be queued')
      },
    })

    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'redis' },
      envVars: [
        { key: 'REDIS_URL', value: 'redis://redis:6379' },
        { key: 'LOG_LEVEL', value: 'info' },
      ],
    })

    assert.equal(result.applicationId, 'app-1')
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/applications', '/variables', '/variables'],
    )
    assert.deepEqual(harness.calls[1].body, {
      organizationId: 'org-1',
      scope: 'APPLICATION',
      applicationId: 'app-1',
      key: 'REDIS_URL',
      value: 'redis://redis:6379',
    })
    assert.equal((harness.calls[2].body as { key: string }).key, 'LOG_LEVEL')
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('saves an application with no variables and does not deploy', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
    })
    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [],
    })
    assert.equal(result.applicationId, 'app-1')
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/applications'],
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

  it('returns the application id after variables are saved and never posts a deployment', async () => {
    const harness = client({
      '/applications': () => ({ application: { id: 'app-1' } }),
      '/variables': () => ({}),
    })
    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [{ key: 'LOG_LEVEL', value: 'info' }],
    })
    assert.equal(result.applicationId, 'app-1')
    assert.equal(harness.calls.filter((call) => call.path === '/variables').length, 1)
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('does not place secret names on the variable endpoint', () => {
    const bodies = applicationVariableBodies('org-1', 'app-1', [
      { key: 'LOG_LEVEL', value: 'info' },
    ])
    assert.equal(JSON.stringify(bodies).includes('SECRET'), false)
    assert.equal(bodies[0].scope, 'APPLICATION')
  })

  it('reuses the application id and does not deploy when variables are already saved', async () => {
    const harness = client({
      '/applications': () => {
        throw new Error('application must be reused')
      },
      '/variables': () => {
        throw new Error('saved variable must not be posted again')
      },
    })
    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      applicationBody: { name: 'api' },
      envVars: [{ key: 'LOG_LEVEL', value: 'info' }],
      resume: { applicationId: 'app-1', savedKeys: ['LOG_LEVEL'] },
    })
    assert.equal(result.applicationId, 'app-1')
    assert.deepEqual(harness.calls, [])
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
        assert.match(err.message, /Press Create application again/)
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
      ],
    )
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('does not post or deploy when a saved variable is removed', async () => {
    const harness = client({
      '/variables': () => ({}),
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
      ],
    )
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('redacts values and keeps demo mode off the production path', () => {
    assert.equal(redactVariableValues('failed for s3cret-value', ['s3cret-value']).includes('s3cret-value'), false)
    assert.equal(creationMode(true, false), 'api')
    assert.equal(creationMode(false, true), 'demo')
    assert.equal(creationMode(false, false), 'blocked')
  })
})

describe('create application storage', () => {
  const volume = { name: 'redis-data', mountPath: '/data', readOnly: false }

  function readyGet(id: string): Record<string, () => unknown> {
    return {
      [`/volumes/${id}`]: () => ({ volume: { id, state: 'READY', name: 'redis-data' } }),
    }
  }

  it('creates, waits, and attaches a volume and returns the application id', async () => {
    const harness = client(
      {
        '/applications': () => ({ application: { id: 'app-1' } }),
        '/volumes': () => ({ volume: { id: 'vol-1', state: 'CREATING' } }),
        '/volumes/vol-1/attach': () => ({ volume: { id: 'vol-1', state: 'ATTACHED' } }),
      },
      readyGet('vol-1'),
    )
    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      serverId: 'server-1',
      applicationBody: { name: 'redis' },
      envVars: [],
      volumes: [volume],
    })
    assert.equal(result.applicationId, 'app-1')
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      [
        '/applications',
        '/volumes',
        '/volumes/vol-1',
        '/volumes/vol-1/attach',
      ],
    )
    assert.deepEqual(harness.calls[1]?.body, {
      organizationId: 'org-1',
      serverId: 'server-1',
      name: 'redis-data',
      mountPath: '/data',
      labels: { readOnly: false },
    })
    assert.deepEqual(harness.calls[3]?.body, {
      resourceType: 'application',
      resourceId: 'app-1',
      mountPath: '/data',
      readOnly: false,
    })
  })

  it('retries attachment without creating another volume', async () => {
    const harness = client(
      {
        '/volumes/vol-1/attach': () => ({}),
      },
      readyGet('vol-1'),
    )
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      serverId: 'server-1',
      applicationBody: { name: 'redis' },
      envVars: [],
      volumes: [volume],
      resume: {
        applicationId: 'app-1',
        savedKeys: [],
        savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: false }],
      },
    })
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/volumes/vol-1', '/volumes/vol-1/attach'],
    )
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('keeps polling a volume that is still creating', async () => {
    let reads = 0
    const harness = client(
      {
        '/volumes/vol-1/attach': () => ({}),
      },
      {
        '/volumes/vol-1': () => {
          reads += 1
          return { volume: { id: 'vol-1', state: reads < 2 ? 'CREATING' : 'READY' } }
        },
      },
    )
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      serverId: 'server-1',
      applicationBody: { name: 'redis' },
      envVars: [],
      volumes: [volume],
      resume: {
        applicationId: 'app-1',
        savedKeys: [],
        savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: false }],
      },
    })
    assert.equal(reads, 2)
    assert.equal(harness.calls.some((call) => call.path === '/volumes'), false)
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('does not deploy when volume creation failed', async () => {
    const harness = client(
      { '/volumes/vol-1/retry': () => ({ volume: { id: 'vol-1', state: 'CREATING' } }) },
      { '/volumes/vol-1': () => ({ volume: { id: 'vol-1', state: 'FAILED' } }) },
    )
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          serverId: 'server-1',
          applicationBody: { name: 'redis' },
          envVars: [],
          volumes: [volume],
          resume: {
            applicationId: 'app-1',
            savedKeys: [],
            savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: false }],
          },
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.equal(err.phase, 'storage')
        assert.match(err.message, /failed to create/)
        return true
      },
    )
    assert.equal(harness.calls.filter((call) => call.path === '/volumes/vol-1/retry').length, 1)
    assert.equal(harness.calls.some((call) => call.path === '/volumes'), false)
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('retries a failed volume on the same id and attaches it without deploying', async () => {
    let reads = 0
    const harness = client(
      {
        '/volumes/vol-1/retry': () => ({ volume: { id: 'vol-1', state: 'CREATING' } }),
        '/volumes/vol-1/attach': () => ({}),
      },
      {
        '/volumes/vol-1': () => {
          reads += 1
          if (reads === 1) return { volume: { id: 'vol-1', state: 'FAILED', dockerName: null } }
          return { volume: { id: 'vol-1', state: 'READY', name: 'redis-data' } }
        },
      },
    )
    await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      serverId: 'server-1',
      applicationBody: { name: 'redis' },
      envVars: [],
      volumes: [volume],
      resume: {
        applicationId: 'app-1',
        savedKeys: [],
        savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: false }],
      },
    })
    assert.deepEqual(
      harness.calls.map((call) => call.path),
      ['/volumes/vol-1', '/volumes/vol-1/retry', '/volumes/vol-1', '/volumes/vol-1/attach'],
    )
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('does not retry or deploy a failed volume that already has a docker name', async () => {
    const harness = client(
      { '/volumes/vol-1/retry': () => ({}) },
      { '/volumes/vol-1': () => ({ volume: { id: 'vol-1', state: 'FAILED', dockerName: 'redis-data' } }) },
    )
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          serverId: 'server-1',
          applicationBody: { name: 'redis' },
          envVars: [],
          volumes: [volume],
          resume: {
            applicationId: 'app-1',
            savedKeys: [],
            savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: false }],
          },
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.equal(err.phase, 'storage')
        return true
      },
    )
    assert.equal(harness.calls.some((call) => call.path === '/volumes/vol-1/retry'), false)
    assert.equal(harness.calls.some((call) => call.path.includes('/deployments')), false)
  })

  it('does not queue a deployment when storage is already attached', async () => {
    const harness = client({})
    const result = await createApplicationWithVariables(harness.client, {
      organizationId: 'org-1',
      serverId: 'server-1',
      applicationBody: { name: 'redis' },
      envVars: [],
      volumes: [volume],
      resume: {
        applicationId: 'app-1',
        savedKeys: [],
        savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: true }],
      },
    })
    assert.equal(result.applicationId, 'app-1')
    assert.deepEqual(harness.calls, [])
  })

  it('does not recreate an earlier volume when a later one fails', async () => {
    const harness = client(
      {
        '/volumes': () => ({ volume: { id: 'vol-2', state: 'CREATING' } }),
      },
      {
        '/volumes/vol-2': () => ({ volume: { id: 'vol-2', state: 'FAILED' } }),
      },
    )
    await assert.rejects(
      () =>
        createApplicationWithVariables(harness.client, {
          organizationId: 'org-1',
          serverId: 'server-1',
          applicationBody: { name: 'redis' },
          envVars: [],
          volumes: [volume, { name: 'cache', mountPath: '/cache', readOnly: true }],
          resume: {
            applicationId: 'app-1',
            savedKeys: [],
            savedVolumes: [{ name: 'redis-data', volumeId: 'vol-1', mountPath: '/data', readOnly: false, attached: true }],
          },
        }),
      (err: unknown) => {
        assert.ok(err instanceof CreateFlowError)
        assert.equal(err.savedVolumes.some((item) => item.volumeId === 'vol-1'), true)
        return true
      },
    )
    assert.equal(harness.calls.filter((call) => call.path === '/volumes').length, 1)
    assert.equal((harness.calls.find((call) => call.path === '/volumes')?.body as { name?: string }).name, 'cache')
  })
})
