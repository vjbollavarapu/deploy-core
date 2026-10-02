import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applicationSecretCreateBody,
  applicationSecretsListPath,
  createApplicationSecret,
  deployActionLabel,
  loadApplicationSecrets,
  manualDeploymentBody,
  manualDeploymentPath,
  mapSecretMetadata,
  queueManualDeployment,
  type BootstrapClient,
} from './application-bootstrap'

const APP_ID = '1d13e36a-a68a-45b1-8380-5556a65e7f25'
const ORG_ID = 'org-1'

describe('application secrets', () => {
  it('creates an application-scoped secret and drops the plaintext afterward', async () => {
    const plaintext = 'super-secret-value'
    const posts: Array<{ path: string; body: unknown }> = []
    const client: BootstrapClient = {
      async get() {
        throw new Error('list is separate')
      },
      async post<T>(path: string, body?: unknown): Promise<T> {
        posts.push({ path, body })
        return {
          secret: {
            id: 'sec-1',
            name: 'SECRET_KEY',
            scope: 'APPLICATION',
            applicationId: APP_ID,
            version: 1,
            value: plaintext,
          },
        } as T
      },
    }

    const result = await createApplicationSecret(client, {
      organizationId: ORG_ID,
      applicationId: APP_ID,
      name: ' SECRET_KEY ',
      value: plaintext,
    })

    assert.deepEqual(posts, [
      {
        path: '/secrets',
        body: applicationSecretCreateBody({
          organizationId: ORG_ID,
          applicationId: APP_ID,
          name: ' SECRET_KEY ',
          value: plaintext,
        }),
      },
    ])
    assert.equal((posts[0].body as { scope: string }).scope, 'APPLICATION')
    assert.equal((posts[0].body as { applicationId: string }).applicationId, APP_ID)
    assert.equal((posts[0].body as { value: string }).value, plaintext)
    assert.equal(result.name, 'SECRET_KEY')
    assert.equal(result.scope, 'APPLICATION')
    assert.equal(result.applicationId, APP_ID)
    assert.equal(result.version, 1)
    assert.equal(result.id, 'sec-1')
    assert.equal('value' in result, false)
    assert.equal(JSON.stringify(result).includes(plaintext), false)
  })

  it('lists application secret metadata without plaintext', async () => {
    const path = applicationSecretsListPath(ORG_ID, APP_ID)
    assert.match(path, /scope=APPLICATION/)
    assert.match(path, new RegExp(`applicationId=${APP_ID}`))
    const client: BootstrapClient = {
      async get<T>(requested: string): Promise<T> {
        assert.equal(requested, path)
        return {
          items: [
            {
              id: 'sec-1',
              name: 'DATABASE_URL',
              scope: 'APPLICATION',
              applicationId: APP_ID,
              version: 2,
              value: 'postgres://should-not-appear',
            },
          ],
        } as T
      },
      async post() {
        throw new Error('list must not create')
      },
    }
    const rows = await loadApplicationSecrets(client, ORG_ID, APP_ID)
    assert.equal(rows.length, 1)
    assert.equal(rows[0].version, 2)
    assert.equal(JSON.stringify(rows).includes('postgres://should-not-appear'), false)
    const mapped = mapSecretMetadata({ name: 'TOKEN', value: 'raw-token' })
    assert.equal(JSON.stringify(mapped).includes('raw-token'), false)
  })
})

describe('explicit deploy', () => {
  it('posts one manual deployment and ignores a second click while pending', async () => {
    assert.equal(deployActionLabel('draft'), 'Deploy')
    assert.equal(deployActionLabel('running'), 'Redeploy')
    assert.deepEqual(manualDeploymentBody(), { trigger: 'manual' })

    let release: () => void = () => {}
    const started = new Promise<void>((resolve) => {
      release = resolve
    })
    const posts: string[] = []
    const client: BootstrapClient = {
      async get() {
        throw new Error('unused')
      },
      async post<T>(path: string, body?: unknown): Promise<T> {
        posts.push(path)
        assert.deepEqual(body, { trigger: 'manual' })
        await started
        return { deployment: { id: 'dep-1' } } as T
      },
    }
    const inflight = { current: false }
    const first = queueManualDeployment(client, APP_ID, inflight)
    const second = queueManualDeployment(client, APP_ID, inflight)
    assert.equal(posts.length, 1)
    assert.equal(posts[0], manualDeploymentPath(APP_ID))
    release()
    const [created, ignored] = await Promise.all([first, second])
    assert.equal(created.kind, 'ok')
    if (created.kind === 'ok') assert.equal(created.deploymentId, 'dep-1')
    assert.deepEqual(ignored, { kind: 'ignored' })
    assert.equal(posts.length, 1)
    assert.equal(inflight.current, false)
  })
})
