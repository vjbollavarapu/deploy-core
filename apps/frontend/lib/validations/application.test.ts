import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  createApplicationSchema,
  DEFAULT_APPLICATION_VALUES,
  healthCheckRequest,
  healthCheckSummary,
  networkingStepSchema,
  storageStepSchema,
  storageSummary,
} from './application'

const redisPlacement = {
  ...DEFAULT_APPLICATION_VALUES,
  sourceType: 'docker-image' as const,
  image: 'redis',
  imageTag: '7-alpine',
  applicationType: 'Docker Image' as const,
  port: 6379,
  domain: '',
  healthCheckPort: 6379,
  name: 'redis',
  projectId: 'project-1',
  environment: 'production',
  serverId: 'server-1',
}

describe('wizard health check path', () => {
  it('accepts a blank path and does not invent one', () => {
    const networking = networkingStepSchema.safeParse({
      domain: '',
      healthCheckPath: '   ',
      healthCheckPort: 6379,
    })
    assert.equal(networking.success, true)
    if (!networking.success) return
    assert.equal(networking.data.healthCheckPath, '')

    const created = createApplicationSchema.safeParse({
      ...redisPlacement,
      healthCheckPath: '',
    })
    assert.equal(created.success, true, JSON.stringify(created.error?.issues))
    assert.deepEqual(healthCheckRequest('', 6379), { enabled: false })
    assert.equal('path' in healthCheckRequest('  ', 6379), false)
    assert.equal(healthCheckSummary('', 6379), 'Disabled')
  })

  it('preserves a configured HTTP health check', () => {
    const networking = networkingStepSchema.safeParse({
      domain: 'api.example.com',
      healthCheckPath: ' /healthz ',
      healthCheckPort: 8080,
    })
    assert.equal(networking.success, true)
    if (!networking.success) return
    assert.equal(networking.data.healthCheckPath, '/healthz')
    assert.deepEqual(healthCheckRequest(networking.data.healthCheckPath, networking.data.healthCheckPort), {
      path: '/healthz',
      port: 8080,
    })
    assert.equal(healthCheckSummary('/healthz', 8080), '/healthz :8080')

    const created = createApplicationSchema.safeParse({
      ...DEFAULT_APPLICATION_VALUES,
      sourceType: 'git',
      repository: 'github.com/acme/api',
      name: 'api',
      projectId: 'project-1',
      environment: 'production',
      serverId: 'server-1',
      healthCheckPath: '/healthz',
    })
    assert.equal(created.success, true, JSON.stringify(created.error?.issues))
  })
})

describe('wizard storage', () => {
  it('accepts no volumes and a writable redis-data mount', () => {
    const empty = storageStepSchema.safeParse({ volumes: [] })
    assert.equal(empty.success, true)
    assert.equal(storageSummary([]), 'None')

    const storage = storageStepSchema.safeParse({
      volumes: [{ name: 'redis-data', mountPath: '/data', writable: true }],
    })
    assert.equal(storage.success, true)
    assert.equal(storageSummary([{ name: 'redis-data', mountPath: '/data', writable: true }]), 'redis-data → /data → Writable')

    const created = createApplicationSchema.safeParse({
      ...redisPlacement,
      healthCheckPath: '',
      volumes: [{ name: 'redis-data', mountPath: '/data', writable: true }],
    })
    assert.equal(created.success, true, JSON.stringify(created.error?.issues))
  })

  it('rejects a relative mount path', () => {
    const storage = storageStepSchema.safeParse({
      volumes: [{ name: 'redis-data', mountPath: 'data', writable: false }],
    })
    assert.equal(storage.success, false)
  })
})
