import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applicationInternalPort,
  applicationsListPath,
  buildAddDomainDefaults,
  createDomainBody,
  createDomainPath,
  createProductionDomain,
  domainApplicationOptions,
  domainCreateErrorMessage,
  domainsListPath,
  loadProductionDomains,
  serversListPath,
  type DomainReadClient,
} from './domain-read'

const ORG_ID = '9c1b7e2a-4f31-4c0a-9a55-2b6d8e0f11aa'
const APP_ID = '1d13e36a-a68a-45b1-8380-5556a65e7f25'
const OTHER_APP_ID = '742c2b91-80b8-4a8e-93ba-82ee7480d285'

describe('production domains', () => {
  it('scopes domain and application lists to the encoded organization', () => {
    const org = 'org/with space'
    assert.equal(domainsListPath(org), '/domains?organizationId=org%2Fwith%20space')
    assert.equal(applicationsListPath(org), '/applications?organizationId=org%2Fwith%20space')
    assert.equal(serversListPath(org), '/servers?organizationId=org%2Fwith%20space')
  })

  it('loads Page items and maps each domain to its application', async () => {
    const calls: string[] = []
    const client: Pick<DomainReadClient, 'get'> = {
      async get<T>(path: string): Promise<T> {
        calls.push(path)
        if (path.startsWith('/domains?')) {
          return {
            domains: [
              {
                id: 'dom-1',
                applicationId: APP_ID,
                hostname: 'api.example.com',
                internalPort: 8000,
                isPrimary: true,
                forceHttps: true,
                dnsStatus: 'VALID',
                tlsStatus: 'ACTIVE',
              },
              {
                id: 'dom-2',
                applicationId: OTHER_APP_ID,
                hostname: 'old.example.com',
                internalPort: 3000,
                dnsStatus: 'PENDING',
                tlsStatus: 'PENDING',
              },
            ],
          } as T
        }
        if (path.startsWith('/applications?')) {
          return {
            items: [
              {
                id: APP_ID,
                name: 'Modulyn',
                targetServerId: 'srv-1',
                config: { internalPort: 8000 },
              },
              { id: '  ', name: 'Skipped' },
              { name: 'Missing id' },
            ],
            limit: 50,
            offset: 0,
          } as T
        }
        return {
          items: [
            {
              id: 'srv-1',
              name: 'oci-validation-01',
              publicIp: '207.211.171.15',
            },
          ],
          limit: 50,
          offset: 0,
        } as T
      },
    }

    const loaded = await loadProductionDomains(client, ORG_ID)
    assert.deepEqual(calls, [
      `/domains?organizationId=${ORG_ID}`,
      `/applications?organizationId=${ORG_ID}`,
      `/servers?organizationId=${ORG_ID}`,
    ])
    assert.deepEqual(loaded.applications, [
      {
        id: APP_ID,
        name: 'Modulyn',
        internalPort: 8000,
        targetServerId: 'srv-1',
        dnsTarget: '207.211.171.15',
      },
    ])
    assert.equal(loaded.domains.length, 2)
    assert.equal(loaded.domains[0]?.application, 'Modulyn')
    assert.equal(loaded.domains[0]?.domain, 'api.example.com')
    assert.equal(loaded.domains[0]?.routingPort, 8000)
    assert.equal(loaded.domains[0]?.status, 'healthy')
    assert.deepEqual(loaded.domains[0]?.requiredRecord, {
      type: 'A',
      name: 'api.example.com',
      value: '207.211.171.15',
    })
    assert.equal(loaded.domains[1]?.application, 'Unknown')
    assert.equal(loaded.domains[1]?.status, 'pending')
  })

  it('treats a missing application page as an empty selector', async () => {
    const client: Pick<DomainReadClient, 'get'> = {
      async get<T>(path: string): Promise<T> {
        if (path.startsWith('/domains?')) return { domains: [] } as T
        return {} as T
      },
    }
    const loaded = await loadProductionDomains(client, ORG_ID)
    assert.deepEqual(loaded, { domains: [], applications: [] })
    assert.deepEqual(domainApplicationOptions(undefined), [])
  })

  it('accepts only integer ports in the container range', () => {
    assert.equal(applicationInternalPort(8000), 8000)
    assert.equal(applicationInternalPort(1), 1)
    assert.equal(applicationInternalPort(65535), 65535)
    assert.equal(applicationInternalPort(0), null)
    assert.equal(applicationInternalPort(65536), null)
    assert.equal(applicationInternalPort(8080.5), null)
    assert.equal(applicationInternalPort(null), null)
    assert.equal(applicationInternalPort('8000'), null)
  })

  it('derives the default routing port from the selected application', () => {
    const applications = [
      {
        id: APP_ID,
        name: 'Modulyn',
        internalPort: 8000,
        targetServerId: 'srv-1',
        dnsTarget: '207.211.171.15',
      },
      {
        id: OTHER_APP_ID,
        name: 'Worker',
        internalPort: null,
        targetServerId: null,
        dnsTarget: null,
      },
    ]
    assert.deepEqual(buildAddDomainDefaults(applications, APP_ID), {
      applicationId: APP_ID,
      routingPort: 8000,
    })
    assert.deepEqual(buildAddDomainDefaults(applications, OTHER_APP_ID), {
      applicationId: OTHER_APP_ID,
      routingPort: 3000,
    })
    assert.deepEqual(buildAddDomainDefaults(applications), {
      applicationId: '',
      routingPort: 3000,
    })
  })

  it('posts hostname, port, primary, and https without an organization id', async () => {
    let seen: { path: string; body?: unknown } | undefined
    const client: Pick<DomainReadClient, 'post'> = {
      async post<T>(path: string, body?: unknown): Promise<T> {
        seen = { path, body }
        return {} as T
      },
    }
    await createProductionDomain(client, {
      applicationId: `${APP_ID}/extra`,
      hostname: 'api.example.com',
      internalPort: 8000,
      isPrimary: false,
      forceHttps: true,
    })
    assert.equal(seen?.path, createDomainPath(`${APP_ID}/extra`))
    assert.deepEqual(seen?.body, createDomainBody({
      applicationId: `${APP_ID}/extra`,
      hostname: 'api.example.com',
      internalPort: 8000,
      isPrimary: false,
      forceHttps: true,
    }))
    assert.equal(seen?.body && typeof seen.body === 'object' && 'organizationId' in seen.body, false)
  })

  it('keeps create failures as errors', () => {
    assert.equal(domainCreateErrorMessage(new Error('hostname already exists')), 'hostname already exists')
    assert.equal(domainCreateErrorMessage('nope'), 'The domain could not be added.')
  })
})
