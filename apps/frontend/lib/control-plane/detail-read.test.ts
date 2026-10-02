import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applicationDeploymentsPath,
  applicationLogsPath,
  applicationRevisionsPath,
  applicationSectionMode,
  applicationVariablesPath,
  loadApplicationDetail,
  loadProductionApplicationRevisions,
  mapApplicationStatus,
  mapProductionApplicationListItem,
  mapRevisionSummary,
  loadDeploymentDetail,
  loadProductionApplication,
  loadProductionApplicationDeployments,
  loadProductionDeployment,
  loadProductionVariables,
  syntheticMarkers,
  toSettingsApplication,
  type ApplicationDetail,
  type DetailClient,
  type DeploymentDetail,
} from './detail-read'

const APP_ID = '1d13e36a-a68a-45b1-8380-5556a65e7f25'
const DEPLOYMENT_ID = '8c0a1b2c-3d4e-5f60-7182-93a4b5c6d7e8'

function client(handlers: Record<string, () => unknown>): DetailClient & { calls: string[] } {
  const calls: string[] = []
  return {
    calls,
    async get<T>(path: string): Promise<T> {
      calls.push(path)
      const handler = handlers[path]
      if (!handler) throw new Error(`unexpected ${path}`)
      return handler() as T
    },
  }
}

function redisApplication() {
  return {
    application: {
      id: APP_ID,
      organizationId: 'org-1',
      projectId: 'proj-1',
      environmentId: 'env-1',
      name: 'redis',
      slug: 'redis',
      type: 'DOCKER_IMAGE',
      status: 'failed',
      targetServerId: 'srv-1',
      config: {
        sourceType: 'image',
        imageReference: 'redis:7-alpine',
        cpuLimitMillis: 500,
        memoryLimitBytes: 268435456,
        runtimeConfig: { desiredReplicas: 1 },
      },
    },
  }
}

function names() {
  return {
    '/projects/proj-1': () => ({ project: { name: 'platform' } }),
    '/environments/env-1': () => ({ environment: { name: 'live' } }),
    '/servers/srv-1': () => ({ server: { name: 'edge-1' } }),
  }
}

describe('production application detail', () => {
  it('loads a UUID that is not a fixture from GET /applications/{id}', async () => {
    const api = client({
      [`/applications/${APP_ID}`]: redisApplication,
      ...names(),
    })
    const result = await loadProductionApplication(api, APP_ID)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(api.calls[0], `/applications/${APP_ID}`)
    assert.equal(result.value.name, 'redis')
    assert.equal(result.value.slug, 'redis')
    assert.equal(result.value.type, 'Docker Image')
    assert.equal(result.value.status, 'failed')
    assert.equal(result.value.imageReference, 'redis:7-alpine')
    assert.equal(result.value.desiredReplicas, 1)
    assert.equal(result.value.projectName, 'platform')
    assert.equal(result.value.environmentName, 'live')
    assert.equal(result.value.serverName, 'edge-1')
    assert.deepEqual(syntheticMarkers(result.value), [])
    assert.equal(result.value.repositoryUrl, null)
    assert.equal(result.value.gitBranch, null)
  })

  it('does not call the API in demo mode and returns the fixture', async () => {
    const fixture = { id: 'app-demo', name: 'Demo' } as ApplicationDetail
    const api = client({})
    const result = await loadApplicationDetail({
      demo: true,
      applicationId: 'app-demo',
      demoApplication: fixture,
      client: api,
    })
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(result.value, fixture)
    assert.deepEqual(api.calls, [])
  })

  it('reports API not-found and network failure', async () => {
    const missing = client({
      [`/applications/${APP_ID}`]: () => {
        const err = new Error('missing') as Error & { status: number }
        err.status = 404
        throw err
      },
    })
    assert.equal((await loadProductionApplication(missing, APP_ID)).kind, 'not-found')

    const down = client({
      [`/applications/${APP_ID}`]: () => {
        throw new Error('network down')
      },
    })
    const failed = await loadProductionApplication(down, APP_ID)
    assert.equal(failed.kind, 'error')
    if (failed.kind === 'error') assert.equal(failed.message, 'network down')
  })
})

describe('production application sections', () => {
  it('queries deployments by application id', async () => {
    const path = applicationDeploymentsPath('org-1', APP_ID)
    assert.match(path, /organizationId=org-1/)
    assert.match(path, new RegExp(`applicationId=${APP_ID}`))
    const api = client({
      [path]: () => ({
        items: [
          {
            id: DEPLOYMENT_ID,
            applicationId: APP_ID,
            status: 'CONTAINER_FAILED',
            trigger: 'manual',
            errorCode: 'DOCKER_ERROR',
            errorMessage: 'No such container: redis-r0-0',
            targetRevisionId: 'rev-real',
          },
        ],
      }),
    })
    const result = await loadProductionApplicationDeployments(api, 'org-1', APP_ID)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(api.calls[0], path)
    assert.equal(result.value[0].status, 'CONTAINER_FAILED')
    assert.equal(result.value[0].errorMessage, 'No such container: redis-r0-0')
    assert.deepEqual(syntheticMarkers(result.value), [])
  })

  it('loads variables for the application', async () => {
    const path = applicationVariablesPath('org-1', APP_ID)
    const api = client({
      [path]: () => ({
        items: [{ id: 'var-1', key: 'REDIS_PORT', value: '6379', scope: 'APPLICATION' }],
      }),
    })
    const result = await loadProductionVariables(api, 'org-1', APP_ID)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(api.calls[0], path)
    assert.equal(result.value[0].key, 'REDIS_PORT')
    assert.equal(result.value[0].value, '6379')
    assert.deepEqual(syntheticMarkers(result.value), [])
  })

  it('renders draft without Unknown and does not invent a revision', () => {
    assert.equal(mapApplicationStatus('draft'), 'draft')
    assert.notEqual(mapApplicationStatus('draft'), 'unknown')
    const row = mapProductionApplicationListItem({
      status: 'draft',
      projectName: 'platform',
      targetServerId: null,
      repositoryUrl: null,
      gitBranch: null,
    })
    assert.equal(row.status, 'draft')
    assert.equal(row.revision, '—')
    assert.equal(row.domain, '—')
    assert.equal(row.lastDeployment, '—')
    assert.equal(row.server, '—')
    assert.deepEqual(syntheticMarkers(row), [])
  })

  it('lists zero revisions as an empty result', async () => {
    const path = applicationRevisionsPath(APP_ID)
    const api = client({ [path]: () => ({ items: [] }) })
    const result = await loadProductionApplicationRevisions(api, APP_ID)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.deepEqual(result.value, [])
  })

  it('keeps revision metadata and drops snapshots', () => {
    const summary = mapRevisionSummary({
      id: 'rev-1',
      revisionNumber: 4,
      status: 'ACTIVE',
      createdAt: '2026-10-02T10:00:00.000Z',
      effectiveConfig: { SECRET_KEY: 'plaintext-secret' },
      variableSnapshot: { DATABASE_URL: 'postgres://secret' },
      secretRefs: [{ name: 'SECRET_KEY', value: 'plaintext-secret' }],
    })
    assert.equal(summary.revisionNumber, 4)
    assert.equal(summary.status, 'ACTIVE')
    assert.equal(JSON.stringify(summary).includes('plaintext-secret'), false)
    assert.equal(JSON.stringify(summary).includes('postgres://secret'), false)
  })

  it('points logs at the application logs endpoint', () => {
    assert.equal(applicationLogsPath(APP_ID), `/applications/${APP_ID}/logs?follow=false`)
  })

  it('keeps volumes on the API panel and settings on the loaded application', async () => {
    assert.equal(applicationSectionMode('volumes', false), 'volumes')
    assert.equal(applicationSectionMode('deployments', false), 'production')
    assert.equal(applicationSectionMode('settings', false), 'production')
    assert.equal(applicationSectionMode('deployments', true), 'demo')
    const api = client({
      [`/applications/${APP_ID}`]: redisApplication,
      ...names(),
    })
    const loaded = await loadProductionApplication(api, APP_ID)
    assert.equal(loaded.kind, 'ok')
    if (loaded.kind !== 'ok') return
    const settings = toSettingsApplication(loaded.value)
    assert.equal(settings.id, APP_ID)
    assert.equal(settings.name, 'redis')
    assert.equal(settings.repo, 'redis:7-alpine')
    assert.equal(settings.instances, '1')
    assert.deepEqual(syntheticMarkers(settings), [])
  })
})

describe('production deployment detail', () => {
  it('loads a failed deployment and keeps its error and events', async () => {
    const api = client({
      [`/deployments/${DEPLOYMENT_ID}`]: () => ({
        deployment: {
          id: DEPLOYMENT_ID,
          applicationId: APP_ID,
          environmentId: 'env-1',
          serverId: 'srv-1',
          status: 'CONTAINER_FAILED',
          trigger: 'manual',
          errorCode: 'DOCKER_ERROR',
          errorMessage: 'inspect container "redis-r0-0": DOCKER_NOT_FOUND',
          targetRevisionId: 'rev-target',
          activeRevisionId: null,
          startedAt: '2026-10-02T10:00:00.000Z',
          finishedAt: '2026-10-02T10:00:12.000Z',
          events: [
            {
              id: 'evt-1',
              fromStatus: 'IMAGE_READY',
              toStatus: 'CREATING_CONTAINER',
              message: 'creating candidate container',
              createdAt: '2026-10-02T10:00:01.000Z',
            },
            {
              id: 'evt-2',
              fromStatus: 'CREATING_CONTAINER',
              toStatus: 'CONTAINER_FAILED',
              message: 'inspect container "redis-r0-0": DOCKER_NOT_FOUND',
              createdAt: '2026-10-02T10:00:12.000Z',
            },
          ],
        },
      }),
      [`/applications/${APP_ID}`]: () => ({ application: { name: 'redis' } }),
      '/environments/env-1': () => ({ environment: { name: 'live' } }),
      '/servers/srv-1': () => ({ server: { name: 'edge-1' } }),
    })
    const result = await loadProductionDeployment(api, DEPLOYMENT_ID)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(result.value.id, DEPLOYMENT_ID)
    assert.equal(result.value.applicationName, 'redis')
    assert.equal(result.value.status, 'CONTAINER_FAILED')
    assert.equal(result.value.errorCode, 'DOCKER_ERROR')
    assert.match(result.value.errorMessage ?? '', /DOCKER_NOT_FOUND/)
    assert.equal(result.value.targetRevisionId, 'rev-target')
    assert.equal(result.value.activeRevisionId, null)
    assert.equal(result.value.serverName, 'edge-1')
    assert.equal(result.value.duration, '12s')
    assert.equal(result.value.events.length, 2)
    assert.equal(result.value.events[1].toStatus, 'CONTAINER_FAILED')
    assert.deepEqual(syntheticMarkers(result.value), [])
  })

  it('uses the demo deployment without calling the API', async () => {
    const fixture = { id: 'dep-demo', status: 'RUNNING' } as DeploymentDetail
    const api = client({})
    const result = await loadDeploymentDetail({
      demo: true,
      deploymentId: 'dep-demo',
      demoDeployment: fixture,
      client: api,
    })
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(result.value, fixture)
    assert.deepEqual(api.calls, [])
  })

  it('reports deployment not-found and network failure', async () => {
    const missing = client({
      [`/deployments/${DEPLOYMENT_ID}`]: () => {
        const err = new Error('missing') as Error & { status: number }
        err.status = 404
        throw err
      },
    })
    assert.equal((await loadProductionDeployment(missing, DEPLOYMENT_ID)).kind, 'not-found')

    const down = client({
      [`/deployments/${DEPLOYMENT_ID}`]: () => {
        throw new Error('network down')
      },
    })
    const failed = await loadProductionDeployment(down, DEPLOYMENT_ID)
    assert.equal(failed.kind, 'error')
    if (failed.kind === 'error') assert.equal(failed.message, 'network down')
  })
})
