import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  createProductionDatabase,
  databaseCreateBody,
  databaseListPath,
  isEnvironmentsCollectionPath,
  loadProductionDatabase,
  loadProductionDatabaseList,
  projectEnvironmentsPath,
  type DatabaseReadClient,
} from './database-read'

const ORG = 'org-1'
const DB = 'db-1'
const SECRET = 'super-secret-value'

function client(handlers: Record<string, (body?: unknown) => unknown>): DatabaseReadClient & { calls: string[] } {
  const calls: string[] = []
  return {
    calls,
    async get<T>(path: string): Promise<T> {
      calls.push(path)
      const handler = handlers[path]
      if (!handler) throw new Error(`unexpected ${path}`)
      return handler() as T
    },
    async post<T>(path: string, body?: unknown): Promise<T> {
      calls.push(path)
      const handler = handlers[path]
      if (!handler) throw new Error(`unexpected ${path}`)
      return handler(body) as T
    },
  }
}

describe('production database list', () => {
  it('requests the active organization and maps Page.items', async () => {
    const listPath = databaseListPath(ORG)
    assert.match(listPath, /organizationId=org-1/)
    const api = client({
      [listPath]: () => ({
        items: [
          {
            id: DB,
            name: 'modulyn',
            engine: 'postgresql',
            engineVersion: '16',
            projectId: 'proj-1',
            environmentId: 'env-1',
            serverId: 'srv-1',
            databaseName: 'app',
            username: 'appuser',
            storageVolumeName: 'db-modulyn-data',
            status: 'PROVISIONING',
            hasCredential: true,
            password: SECRET,
          },
        ],
      }),
      [`/projects?organizationId=${encodeURIComponent(ORG)}`]: () => ({
        items: [{ id: 'proj-1', name: 'platform' }],
      }),
      [`/servers?organizationId=${encodeURIComponent(ORG)}`]: () => ({
        items: [{ id: 'srv-1', name: 'edge-1' }],
      }),
    })

    const result = await loadProductionDatabaseList(api, ORG)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(api.calls[0], listPath)
    assert.equal(result.value.length, 1)
    assert.equal(result.value[0].name, 'modulyn')
    assert.equal(result.value[0].project, 'platform')
    assert.equal(result.value[0].server, 'edge-1')
    assert.equal(result.value[0].environment, 'env-1')
    assert.equal(result.value[0].status, 'pending')
    assert.equal(result.value[0].storageVolumeName, 'db-modulyn-data')
    assert.equal(result.value[0].storageTotalGb, 0)
    assert.equal('password' in result.value[0], false)
    assert.equal(JSON.stringify(result.value).includes(SECRET), false)
    assert.equal(api.calls.some((path) => isEnvironmentsCollectionPath(path)), false)
  })

  it('keeps a successful list when project and server lookups fail', async () => {
    const listPath = databaseListPath(ORG)
    const api = client({
      [listPath]: () => ({
        items: [{ id: DB, name: 'modulyn', projectId: 'proj-1', serverId: 'srv-1', environmentId: 'env-1' }],
      }),
      [`/projects?organizationId=${encodeURIComponent(ORG)}`]: () => {
        throw new Error('projects down')
      },
      [`/servers?organizationId=${encodeURIComponent(ORG)}`]: () => {
        throw new Error('servers down')
      },
    })
    const result = await loadProductionDatabaseList(api, ORG)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(result.value[0].name, 'modulyn')
    assert.equal(result.value[0].project, 'proj-1')
    assert.equal(result.value[0].server, 'srv-1')
    assert.equal(api.calls.some((path) => isEnvironmentsCollectionPath(path)), false)
  })
})

describe('production database create', () => {
  it('posts the real contract and does not synthesize a database or keep a password', async () => {
    const input = {
      organizationId: ORG,
      projectId: 'proj-1',
      environmentId: 'env-1',
      serverId: 'srv-1',
      name: 'modulyn',
      engineVersion: '16',
      databaseName: 'app',
      username: 'appuser',
    }
    const body = databaseCreateBody(input)
    assert.equal(body.engine, 'postgresql')
    assert.equal(body.organizationId, ORG)
    assert.equal(body.environmentId, 'env-1')
    assert.equal(body.serverId, 'srv-1')
    assert.equal('password' in body, false)
    assert.equal(JSON.stringify(body).includes(SECRET), false)

    let posted: unknown
    const api = client({
      '/databases': (requestBody) => {
        posted = requestBody
        return {
          database: {
            id: DB,
            name: 'modulyn',
            password: SECRET,
          },
        }
      },
    })
    const created = await createProductionDatabase(api, input)
    assert.deepEqual(posted, body)
    assert.deepEqual(created, { id: DB })
    assert.equal('password' in created, false)
    assert.equal(JSON.stringify(created).includes(SECRET), false)
    assert.deepEqual(api.calls, ['/databases'])
    assert.equal(projectEnvironmentsPath('proj-1'), '/projects/proj-1/environments')
    assert.equal(isEnvironmentsCollectionPath('/environments'), true)
    assert.equal(isEnvironmentsCollectionPath(projectEnvironmentsPath('proj-1')), false)
  })
})

describe('production database detail', () => {
  it('loads GET /databases/{id} and drops credential material', async () => {
    const api = client({
      [`/databases/${DB}`]: () => ({
        database: {
          id: DB,
          name: 'modulyn',
          engine: 'postgresql',
          engineVersion: '16',
          projectId: 'proj-1',
          environmentId: 'env-1',
          serverId: 'srv-1',
          databaseName: 'app',
          username: 'appuser',
          status: 'RUNNING',
          hasCredential: true,
          privateHost: 'db-modulyn',
          port: 5432,
          password: SECRET,
        },
      }),
      '/projects/proj-1': () => ({ project: { name: 'platform' } }),
      '/environments/env-1': () => ({ environment: { name: 'live' } }),
      '/servers/srv-1': () => ({ server: { name: 'edge-1' } }),
    })
    const result = await loadProductionDatabase(api, DB)
    assert.equal(result.kind, 'ok')
    if (result.kind !== 'ok') return
    assert.equal(api.calls[0], `/databases/${DB}`)
    assert.equal(result.value.project, 'platform')
    assert.equal(result.value.environment, 'live')
    assert.equal(result.value.server, 'edge-1')
    assert.equal(result.value.status, 'running')
    assert.equal(result.value.credentialsRevealAllowed, true)
    assert.equal(result.value.connectionHost, 'db-modulyn')
    assert.equal(result.value.port, 5432)
    assert.equal('password' in result.value, false)
    assert.equal(JSON.stringify(result.value).includes(SECRET), false)
    assert.equal(api.calls.some((path) => isEnvironmentsCollectionPath(path)), false)
  })
})
