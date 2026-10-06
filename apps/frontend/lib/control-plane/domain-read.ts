/** Production domain list and create requests. No mock fallback. */

import type { Application, Domain, Page } from '@/lib/api/contract'
import { DEFAULT_ADD_DOMAIN_VALUES } from '@/lib/validations/domain'
import type { DomainRecord, DomainTlsState } from '@/lib/types'

export interface DomainReadClient {
  get<T>(path: string): Promise<T>
  post<T>(path: string, body?: unknown): Promise<T>
}

export interface DomainApplicationOption {
  id: string
  name: string
  internalPort: number | null
}

export interface CreateProductionDomainInput {
  applicationId: string
  hostname: string
  internalPort: number
  isPrimary: boolean
  forceHttps: boolean
}

const DOMAIN_TLS_STATES = new Set<DomainTlsState>([
  'PENDING',
  'VERIFYING',
  'ISSUING',
  'ACTIVE',
  'EXPIRING',
  'FAILED',
])

export function domainsListPath(organizationId: string): string {
  return `/domains?organizationId=${encodeURIComponent(organizationId)}`
}

export function applicationsListPath(organizationId: string): string {
  return `/applications?organizationId=${encodeURIComponent(organizationId)}`
}

export function createDomainPath(applicationId: string): string {
  return `/applications/${encodeURIComponent(applicationId)}/domains`
}

export function applicationInternalPort(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 1 || value > 65535) {
    return null
  }
  return value
}

export function domainApplicationOptions(
  items: Application[] | null | undefined,
): DomainApplicationOption[] {
  if (!Array.isArray(items)) return []
  const options: DomainApplicationOption[] = []
  for (const item of items) {
    const id = item.id?.trim()
    if (!id) continue
    options.push({
      id,
      name: item.name?.trim() || 'Untitled',
      internalPort: applicationInternalPort(item.config?.internalPort),
    })
  }
  return options
}

export function buildAddDomainDefaults(
  applications: readonly DomainApplicationOption[],
  defaultApplicationId?: string,
): { applicationId: string; routingPort: number } {
  const applicationId = defaultApplicationId ?? ''
  const selected = applications.find((application) => application.id === applicationId)
  return {
    applicationId,
    routingPort: selected?.internalPort ?? DEFAULT_ADD_DOMAIN_VALUES.routingPort,
  }
}

export function createDomainBody(input: CreateProductionDomainInput) {
  return {
    hostname: input.hostname,
    internalPort: input.internalPort,
    isPrimary: input.isPrimary,
    forceHttps: input.forceHttps,
  }
}

export async function createProductionDomain(
  client: Pick<DomainReadClient, 'post'>,
  input: CreateProductionDomainInput,
): Promise<void> {
  await client.post(createDomainPath(input.applicationId), createDomainBody(input))
}

export function domainCreateErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) return err.message.trim()
  return 'The domain could not be added.'
}

export function mapProductionDomainRecords(
  domains: Domain[] | null | undefined,
  applications: readonly DomainApplicationOption[],
): DomainRecord[] {
  const names = new Map(applications.map((application) => [application.id, application.name]))
  if (!Array.isArray(domains)) return []
  return domains.map((domain) => {
    const applicationId = domain.applicationId ?? ''
    const hostname = domain.hostname ?? ''
    return {
      id: domain.id ?? '',
      domain: hostname,
      applicationId,
      application: names.get(applicationId) || 'Unknown',
      environment: 'production',
      routingPort: domain.internalPort ?? 0,
      dnsVerified: domain.dnsStatus === 'VALID',
      https: domain.forceHttps === true,
      certExpiry: 'N/A',
      certExpiryDays: 0,
      status: domain.tlsStatus === 'ACTIVE' && domain.dnsStatus === 'VALID' ? 'healthy' : 'pending',
      tlsState: toDomainTlsState(domain.tlsStatus),
      certificateIssuer: null,
      certificateIssuedAt: null,
      nextRenewalAt: null,
      lastValidatedAt: null,
      validationMessage: '',
      primary: domain.isPrimary === true,
      forceHttps: domain.forceHttps === true,
      requiredRecord: { type: 'CNAME', name: hostname, value: 'proxy.deploycore.io' },
      detectedRecord: null,
      redirectRules: [],
    }
  })
}

export async function loadProductionDomains(
  client: Pick<DomainReadClient, 'get'>,
  organizationId: string,
): Promise<{ domains: DomainRecord[]; applications: DomainApplicationOption[] }> {
  const [domainResponse, applicationResponse] = await Promise.all([
    client.get<{ domains?: Domain[] | null }>(domainsListPath(organizationId)),
    client.get<Page<Application>>(applicationsListPath(organizationId)),
  ])
  const applications = domainApplicationOptions(applicationResponse?.items)
  return {
    applications,
    domains: mapProductionDomainRecords(domainResponse?.domains, applications),
  }
}

function toDomainTlsState(value: string | undefined): DomainTlsState {
  if (value && DOMAIN_TLS_STATES.has(value as DomainTlsState)) return value as DomainTlsState
  return 'PENDING'
}
