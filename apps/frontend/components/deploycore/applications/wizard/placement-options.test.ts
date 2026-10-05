import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  environmentOptionLabel,
  environmentSelectItems,
  placementAfterProjectChange,
  placementReadyForOrganization,
  projectSelectItems,
  serverOptionLabel,
  serverSelectItems,
  type PlacementProjectOption,
  type PlacementServerOption,
} from './placement-options'

const projectId = '2c15ea3b-dd1c-49e6-8ee7-c97457bf6e30'
const environmentId = '7ac31add-47eb-4a4f-94d5-c53604ace44d'
const otherEnvironmentId = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'
const serverId = '11111111-2222-3333-4444-555555555555'

const projects: PlacementProjectOption[] = [
  {
    id: projectId,
    name: 'payments',
    environments: [
      { id: environmentId, name: 'production', kind: 'production' },
      { id: otherEnvironmentId, name: 'staging', kind: 'staging' },
    ],
  },
  {
    id: '99999999-8888-7777-6666-555555555555',
    name: 'billing',
    environments: [{ id: '12121212-3434-5656-7878-909090909090', name: 'production', kind: 'production' }],
  },
]

const servers: PlacementServerOption[] = [
  { id: serverId, name: 'oci-validation-01', region: 'Oracle Cloud', status: 'online' },
  { id: 'offline-server', name: 'retired-01', region: 'Oracle Cloud', status: 'OFFLINE' },
]

describe('placement dropdown labels', () => {
  it('shows project names for every project option and for the selected value', () => {
    const items = projectSelectItems(projects)
    assert.deepEqual(Object.keys(items), [projectId, '99999999-8888-7777-6666-555555555555'])
    assert.equal(items[projectId], 'payments')
    assert.notEqual(items[projectId], projectId)
    assert.equal(items['99999999-8888-7777-6666-555555555555'], 'billing')
  })

  it('shows environment names scoped to the selected project', () => {
    const items = environmentSelectItems(projects, projectId)
    assert.deepEqual(Object.keys(items), [environmentId, otherEnvironmentId])
    assert.equal(items[environmentId], environmentOptionLabel(projects[0].environments[0]))
    assert.equal(items[environmentId], 'production (production)')
    assert.equal(items[otherEnvironmentId], 'staging (staging)')
    assert.equal(environmentSelectItems(projects, '99999999-8888-7777-6666-555555555555')['12121212-3434-5656-7878-909090909090'], 'production (production)')
    assert.equal(environmentSelectItems(projects, projectId)['12121212-3434-5656-7878-909090909090'], undefined)
  })

  it('shows a human-readable server label and hides offline servers', () => {
    const items = serverSelectItems(servers)
    assert.deepEqual(Object.keys(items), [serverId])
    assert.equal(items[serverId], serverOptionLabel(servers[0]))
    assert.equal(items[serverId], 'oci-validation-01 · Oracle Cloud')
    assert.equal(items['offline-server'], undefined)
  })

  it('clears the environment and keeps the server when the project changes', () => {
    const next = placementAfterProjectChange(serverId)
    assert.equal(next.environment, '')
    assert.equal(next.serverId, serverId)
    assert.equal(environmentSelectItems(projects, '99999999-8888-7777-6666-555555555555')[environmentId], undefined)
  })

  it('rejects placement loaded for another organization', () => {
    const ready = {
      activeOrganizationId: 'org-b',
      loadedOrganizationId: 'org-b',
      projectId,
      environment: environmentId,
      serverId,
      projects,
      servers,
    }
    assert.equal(placementReadyForOrganization(ready), true)
    assert.equal(placementReadyForOrganization({ ...ready, loadedOrganizationId: 'org-a' }), false)
    assert.equal(placementReadyForOrganization({ ...ready, loadedOrganizationId: '' }), false)
    assert.equal(placementReadyForOrganization({ ...ready, projectId: 'missing-project' }), false)
  })
})
