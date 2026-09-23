import { apiClient, type Page } from '@/lib/api'
import type {
  GitProviderConnection,
  GitProviderType,
  NotificationChannel,
  NotificationChannelType,
  NotificationPolicy,
  Registry,
  RegistryType,
  Status,
  Webhook,
  WebhookDelivery,
} from '@/lib/types'

export const GIT_PROVIDER_TYPES: GitProviderType[] = [
  'GitHub',
  'GitLab',
  'Bitbucket',
  'Generic Git',
]

export const REGISTRY_TYPE_LABELS: Record<RegistryType, string> = {
  'GitHub Container Registry': 'GHCR',
  'Docker Hub': 'Docker Hub',
  'AWS ECR': 'ECR',
  'GCP Artifact Registry': 'GCP Artifact Registry',
  'Azure Container Registry': 'ACR',
  'Generic OCI Registry': 'Generic OCI',
}

export const REGISTRY_PROVIDER_KEYS: Record<string, RegistryType> = {
  ghcr: 'GitHub Container Registry',
  dockerhub: 'Docker Hub',
  ecr: 'AWS ECR',
  gcp: 'GCP Artifact Registry',
  acr: 'Azure Container Registry',
  oci: 'Generic OCI Registry',
}

export const WEBHOOK_EVENT_OPTIONS = [
  'deployment.started',
  'deployment.completed',
  'deployment.failed',
  'server.offline',
  'server.degraded',
  'backup.completed',
  'backup.failed',
  'certificate.expiring',
] as const

export function isIntegrationConnected(status: string): boolean {
  return status === 'healthy' || status === 'running' || status === 'active' || status === 'ACTIVE'
}

/* ==========================================================================
   Wire Models & Mappers
   ========================================================================== */

export interface WireGitConnection {
  id: string
  organizationId: string
  provider: string
  accountLogin: string
  displayName: string
  status: string
  lastSyncAt?: string | null
  metadata?: Record<string, unknown>
  createdAt: string
  updatedAt: string
  hasWebhookSecret: boolean
}

export interface WireGitRepository {
  id: string
  organizationId: string
  connectionId: string
  externalId: string
  fullName: string
  defaultBranch: string
  cloneUrl: string
  htmlUrl: string
  metadata?: Record<string, unknown>
  lastSyncAt?: string | null
  createdAt: string
  updatedAt: string
}

export function mapWireGitConnection(wire: WireGitConnection, repoCount = 0): GitProviderConnection {
  let providerType: GitProviderType = 'Generic Git'
  switch (wire.provider.toLowerCase()) {
    case 'github':
      providerType = 'GitHub'
      break
    case 'gitlab':
      providerType = 'GitLab'
      break
    case 'bitbucket':
      providerType = 'Bitbucket'
      break
    default:
      providerType = 'Generic Git'
      break
  }

  const status: Status =
    wire.status === 'active'
      ? 'healthy'
      : wire.status === 'error'
        ? 'failed'
        : wire.status === 'revoked'
          ? 'stopped'
          : 'stopped'

  const orgs = Array.isArray(wire.metadata?.organizations)
    ? (wire.metadata.organizations as string[])
    : [wire.accountLogin || 'default']

  const perms = Array.isArray(wire.metadata?.permissions)
    ? (wire.metadata.permissions as string[])
    : ['read:repo', 'read:org']

  return {
    id: wire.id,
    type: providerType,
    account: wire.accountLogin || wire.displayName || 'Connected Account',
    organizations: orgs,
    repositoryCount: repoCount || (wire.metadata?.repositoryCount as number) || 0,
    status,
    permissions: perms,
    lastSync: wire.lastSyncAt ? new Date(wire.lastSyncAt).toLocaleString() : 'Never',
  }
}

export interface WireRegistry {
  id: string
  organizationId: string
  name: string
  provider: string
  registryUrl: string
  username: string
  status: string
  metadata?: Record<string, unknown>
  hasCredentials: boolean
  createdAt: string
  updatedAt: string
}

export function mapWireRegistry(wire: WireRegistry): Registry {
  const regType: RegistryType =
    REGISTRY_PROVIDER_KEYS[wire.provider.toLowerCase()] ?? 'Generic OCI Registry'

  const status: Status =
    wire.status === 'active'
      ? 'healthy'
      : wire.status === 'error'
        ? 'failed'
        : wire.status === 'disabled'
          ? 'stopped'
          : 'unknown'

  return {
    id: wire.id,
    name: wire.name,
    type: regType,
    url: wire.registryUrl,
    imageCount: (wire.metadata?.imageCount as number) || 0,
    connectedAt: wire.createdAt ? new Date(wire.createdAt).toLocaleDateString() : '—',
    status,
  }
}

export interface WireNotificationChannel {
  id: string
  organizationId: string
  name: string
  type: string
  config: Record<string, unknown>
  enabled: boolean
  status: string
  lastError?: string
  hasCredential?: boolean
  createdAt: string
  updatedAt: string
}

export function mapWireNotificationChannel(wire: WireNotificationChannel): NotificationChannel {
  let channelType: NotificationChannelType = 'Webhook'
  switch (wire.type.toUpperCase()) {
    case 'EMAIL':
      channelType = 'Email'
      break
    case 'SLACK':
      channelType = 'Slack'
      break
    case 'TEAMS':
      channelType = 'Microsoft Teams'
      break
    case 'DISCORD':
      channelType = 'Discord'
      break
    case 'TELEGRAM':
      channelType = 'Telegram'
      break
    case 'WHATSAPP':
      channelType = 'WhatsApp'
      break
    default:
      channelType = 'Webhook'
      break
  }

  const target =
    (wire.config?.email as string) ||
    (wire.config?.webhookUrl as string) ||
    (wire.config?.target as string) ||
    wire.name

  const status: Status = !wire.enabled
    ? 'stopped'
    : wire.status === 'ACTIVE'
      ? 'healthy'
      : wire.status === 'FAILED'
        ? 'failed'
        : 'unknown'

  return {
    id: wire.id,
    name: wire.name,
    type: channelType,
    target,
    status,
  }
}

export interface WireNotificationPolicy {
  id: string
  organizationId: string
  name: string
  eventTypes: string[]
  resourceFilters?: Record<string, unknown>
  environmentFilters?: Record<string, unknown>
  channelIds: string[]
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export function mapWireNotificationPolicy(
  wire: WireNotificationPolicy,
  channelNameMap: Record<string, string> = {},
): NotificationPolicy {
  const channelNames = wire.channelIds.map((id) => channelNameMap[id] || id)

  return {
    id: wire.id,
    name: wire.name,
    when: wire.eventTypes.join(', '),
    conditions: wire.eventTypes.map((e) => e.replace(/_/g, ' ').toLowerCase()),
    channels: channelNames,
    enabled: wire.enabled,
  }
}

export interface WireWebhookDelivery {
  id: string
  organizationId: string
  webhookId: string
  eventType: string
  payload?: Record<string, unknown>
  status: string
  attemptCount: number
  responseCode?: number | null
  latencyMs?: number | null
  lastError?: string
  deliveredAt?: string | null
  createdAt: string
  updatedAt: string
}

export function mapWireWebhookDelivery(wire: WireWebhookDelivery): WebhookDelivery {
  return {
    id: wire.id,
    timestamp: wire.deliveredAt
      ? new Date(wire.deliveredAt).toLocaleTimeString()
      : new Date(wire.createdAt).toLocaleTimeString(),
    statusCode: wire.responseCode ?? (wire.status === 'DELIVERED' ? 200 : 500),
    latencyMs: wire.latencyMs ?? 0,
    request: JSON.stringify(wire.payload || {}, null, 2),
    response: wire.lastError ? `Error: ${wire.lastError}` : `Status: ${wire.status}`,
    retried: wire.attemptCount > 1,
    attempts: wire.attemptCount || 1,
  }
}

export interface WireWebhook {
  id: string
  organizationId: string
  name: string
  url: string
  events: string[]
  enabled: boolean
  status: string
  consecutiveFailures: number
  failureThreshold: number
  lastError?: string
  createdAt: string
  updatedAt: string
}

export function mapWireWebhook(wire: WireWebhook, deliveries: WebhookDelivery[] = []): Webhook {
  const status: Status = !wire.enabled
    ? 'stopped'
    : wire.status === 'ACTIVE'
      ? 'healthy'
      : wire.status === 'FAILING'
        ? 'failed'
        : 'stopped'

  const latestDelivery = deliveries.length > 0 ? deliveries[0].timestamp : 'No deliveries yet'

  return {
    id: wire.id,
    name: wire.name,
    endpoint: wire.url,
    events: wire.events,
    enabled: wire.enabled,
    status,
    latestDelivery,
    signingSecretMasked: 'whsec_••••••••••••••••••••••••',
    deliveries,
  }
}

/* ==========================================================================
   API Client Helper Functions
   ========================================================================== */

// Git connections
export async function fetchGitConnections(organizationId: string) {
  return apiClient.get<Page<WireGitConnection>>(`/integrations/git/connections?organizationId=${organizationId}`)
}

export async function createGitConnection(data: {
  organizationId: string
  provider: string
  accountLogin: string
  displayName: string
  accessToken: string
  webhookSecret?: string
}) {
  return apiClient.post<WireGitConnection>('/integrations/git/connections', data)
}

export async function syncGitConnection(connectionId: string) {
  return apiClient.post<{ message: string; syncedAt: string }>(`/integrations/git/connections/${connectionId}/sync`)
}

export async function fetchGitRepositories(connectionId: string) {
  return apiClient.get<Page<WireGitRepository>>(`/integrations/git/connections/${connectionId}/repositories`)
}

export async function deleteGitConnection(connectionId: string) {
  return apiClient.delete(`/integrations/git/connections/${connectionId}`)
}

// Registries
export async function fetchRegistries(organizationId: string) {
  return apiClient.get<Page<WireRegistry>>(`/integrations/registries?organizationId=${organizationId}`)
}

export async function createRegistry(data: {
  organizationId: string
  name: string
  provider: string
  registryUrl: string
  username: string
  credentials: { username: string; token: string }
}) {
  return apiClient.post<WireRegistry>('/integrations/registries', data)
}

export async function deleteRegistry(registryId: string) {
  return apiClient.delete(`/integrations/registries/${registryId}`)
}

// Notifications
export async function fetchNotificationChannels(organizationId: string) {
  return apiClient.get<Page<WireNotificationChannel>>(`/integrations/notifications/channels?organizationId=${organizationId}`)
}

export async function createNotificationChannel(data: {
  organizationId: string
  name: string
  type: string
  config: Record<string, unknown>
  credential?: string
  enabled?: boolean
}) {
  return apiClient.post<WireNotificationChannel>('/integrations/notifications/channels', data)
}

export async function updateNotificationChannel(channelId: string, data: Partial<{
  name: string
  config: Record<string, unknown>
  credential: string
  enabled: boolean
  status: string
}>) {
  return apiClient.patch<WireNotificationChannel>(`/integrations/notifications/channels/${channelId}`, data)
}

export async function deleteNotificationChannel(channelId: string) {
  return apiClient.delete(`/integrations/notifications/channels/${channelId}`)
}

export async function fetchNotificationPolicies(organizationId: string) {
  return apiClient.get<Page<WireNotificationPolicy>>(`/integrations/notifications/policies?organizationId=${organizationId}`)
}

export async function createNotificationPolicy(data: {
  organizationId: string
  name: string
  eventTypes: string[]
  channelIds: string[]
  enabled?: boolean
}) {
  return apiClient.post<WireNotificationPolicy>('/integrations/notifications/policies', data)
}

export async function updateNotificationPolicy(policyId: string, data: Partial<{
  name: string
  eventTypes: string[]
  channelIds: string[]
  enabled: boolean
}>) {
  return apiClient.patch<WireNotificationPolicy>(`/integrations/notifications/policies/${policyId}`, data)
}

export async function deleteNotificationPolicy(policyId: string) {
  return apiClient.delete(`/integrations/notifications/policies/${policyId}`)
}

export async function emitNotificationTest(organizationId: string, eventType: string, payload: Record<string, unknown>) {
  return apiClient.post<{ deliveriesCreated: number }>('/integrations/notifications/emit', {
    organizationId,
    eventType,
    payload,
  })
}

// Webhooks
export async function fetchWebhooks(organizationId: string) {
  return apiClient.get<Page<WireWebhook>>(`/integrations/webhooks?organizationId=${organizationId}`)
}

export async function createWebhook(data: {
  organizationId: string
  name: string
  url: string
  events: string[]
  secret?: string
  enabled?: boolean
  failureThreshold?: number
}) {
  return apiClient.post<WireWebhook & { secretPlain?: string }>('/integrations/webhooks', data)
}

export async function updateWebhook(webhookId: string, data: Partial<{
  name: string
  url: string
  events: string[]
  enabled: boolean
  failureThreshold: number
  rotateSecret: boolean
}>) {
  return apiClient.patch<WireWebhook & { secretPlain?: string }>(`/integrations/webhooks/${webhookId}`, data)
}

export async function deleteWebhook(webhookId: string) {
  return apiClient.delete(`/integrations/webhooks/${webhookId}`)
}

export async function fetchWebhookDeliveries(webhookId: string) {
  return apiClient.get<Page<WireWebhookDelivery>>(`/integrations/webhooks/${webhookId}/deliveries`)
}

export async function emitWebhookTest(organizationId: string, eventType: string, payload: Record<string, unknown>) {
  return apiClient.post<{ deliveredCount: number }>('/integrations/webhooks/emit', {
    organizationId,
    eventType,
    payload,
  })
}
