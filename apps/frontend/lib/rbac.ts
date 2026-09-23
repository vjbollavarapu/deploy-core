import { apiClient } from '@/lib/api/client'
import type { Page } from '@/lib/api/contract'
import type { AuditLogEntry, Invitation, PermissionLevel, TeamMember } from '@/lib/types'

export const DEFAULT_ROLES = [
  {
    name: 'Owner',
    key: 'owner',
    tier: 'Organization Owner',
    description: 'Full control of the organization, billing, clusters, and membership.',
    capabilityCount: 45,
  },
  {
    name: 'Administrator',
    key: 'administrator',
    tier: 'Administrator',
    description: 'Manage infrastructure, applications, and team members; cannot delete organization.',
    capabilityCount: 44,
  },
  {
    name: 'DevOps',
    key: 'devops',
    tier: 'Infrastructure',
    description: 'Operate servers, deployments, databases, secrets, and automated backups.',
    capabilityCount: 34,
  },
  {
    name: 'Developer',
    key: 'developer',
    tier: 'Engineering',
    description: 'Deploy and configure applications and services within assigned projects.',
    capabilityCount: 24,
  },
  {
    name: 'Support',
    key: 'support',
    tier: 'Operations',
    description: 'Read-mostly access for troubleshooting and incident mitigation.',
    capabilityCount: 14,
  },
  {
    name: 'Viewer',
    key: 'viewer',
    tier: 'Read Only',
    description: 'Read-only visibility into project workloads, metrics, and runtime status.',
    capabilityCount: 12,
  },
] as const

export type DefaultRoleName = (typeof DEFAULT_ROLES)[number]['name']

export interface CapabilityDefinition {
  key: string
  name: string
  description: string
  category: string
}

export const CAPABILITY_CATEGORIES = [
  'Organization & IAM',
  'Infrastructure & Servers',
  'Projects & Environments',
  'Applications & Workloads',
  'Deployments',
  'Databases & Storage',
  'Secrets & Credentials',
  'Networking & Domains',
  'Integrations & Notifications',
  'Observability & Compliance',
] as const

export const CAPABILITIES: CapabilityDefinition[] = [
  // Organization & IAM
  { key: 'organization.read', name: 'View Organization', description: 'View organization profile and settings', category: 'Organization & IAM' },
  { key: 'organization.update', name: 'Update Organization', description: 'Modify organization name and settings', category: 'Organization & IAM' },
  { key: 'organization.delete', name: 'Delete Organization', description: 'Delete or schedule organization deletion', category: 'Organization & IAM' },
  { key: 'member.read', name: 'View Members', description: 'List organization members and roles', category: 'Organization & IAM' },
  { key: 'member.invite', name: 'Invite Members', description: 'Send and revoke member invitations', category: 'Organization & IAM' },
  { key: 'member.update', name: 'Update Member Roles', description: 'Modify member role assignments', category: 'Organization & IAM' },
  { key: 'member.remove', name: 'Remove Members', description: 'Remove members from organization', category: 'Organization & IAM' },

  // Infrastructure & Servers
  { key: 'server.read', name: 'View Servers', description: 'View registered nodes and cluster hosts', category: 'Infrastructure & Servers' },
  { key: 'server.create', name: 'Register Servers', description: 'Connect new host servers and install node agents', category: 'Infrastructure & Servers' },
  { key: 'server.update', name: 'Update Servers', description: 'Modify server configuration and drain nodes', category: 'Infrastructure & Servers' },
  { key: 'server.delete', name: 'Delete Servers', description: 'Deregister and detach host servers', category: 'Infrastructure & Servers' },

  // Projects & Environments
  { key: 'project.read', name: 'View Projects', description: 'View projects and environments', category: 'Projects & Environments' },
  { key: 'project.create', name: 'Create Projects', description: 'Create new project workspaces', category: 'Projects & Environments' },
  { key: 'project.update', name: 'Update Projects', description: 'Update project settings and environment stages', category: 'Projects & Environments' },
  { key: 'project.delete', name: 'Delete Projects', description: 'Delete projects and scoped resources', category: 'Projects & Environments' },

  // Applications & Workloads
  { key: 'application.read', name: 'View Applications', description: 'View applications and configuration', category: 'Applications & Workloads' },
  { key: 'application.create', name: 'Create Applications', description: 'Create and configure new applications', category: 'Applications & Workloads' },
  { key: 'application.update', name: 'Update Applications', description: 'Update application settings, ports, and build params', category: 'Applications & Workloads' },
  { key: 'application.deploy', name: 'Deploy Applications', description: 'Trigger new container deployments', category: 'Applications & Workloads' },
  { key: 'application.restart', name: 'Restart Applications', description: 'Restart containers and workload pods', category: 'Applications & Workloads' },
  { key: 'application.stop', name: 'Stop Applications', description: 'Stop active application workloads', category: 'Applications & Workloads' },
  { key: 'application.delete', name: 'Delete Applications', description: 'Delete applications and runtime containers', category: 'Applications & Workloads' },

  // Deployments
  { key: 'deployment.read', name: 'View Deployments', description: 'View deployment history and live build logs', category: 'Deployments' },
  { key: 'deployment.create', name: 'Trigger Deployments', description: 'Queue manual application deployments', category: 'Deployments' },
  { key: 'deployment.cancel', name: 'Cancel Deployments', description: 'Abort queued or running deployments', category: 'Deployments' },
  { key: 'deployment.rollback', name: 'Rollback Deployments', description: 'Roll back workloads to previous stable revisions', category: 'Deployments' },

  // Databases & Storage
  { key: 'database.read', name: 'View Databases', description: 'View managed databases and engine telemetry', category: 'Databases & Storage' },
  { key: 'database.create', name: 'Provision Databases', description: 'Provision PostgreSQL, MySQL, Redis, and Mongo instances', category: 'Databases & Storage' },
  { key: 'database.update', name: 'Update Databases', description: 'Scale database resources and configuration', category: 'Databases & Storage' },
  { key: 'database.backup', name: 'Create Backups', description: 'Trigger snapshots and database backups', category: 'Databases & Storage' },
  { key: 'database.restore', name: 'Restore Backups', description: 'Restore databases from snapshot archives', category: 'Databases & Storage' },

  // Secrets & Credentials
  { key: 'secret.read_metadata', name: 'View Secret Metadata', description: 'View secret names and scopes (values remain sealed)', category: 'Secrets & Credentials' },
  { key: 'secret.create', name: 'Create Secrets', description: 'Write envelope-encrypted secrets and variables', category: 'Secrets & Credentials' },
  { key: 'secret.update', name: 'Update Secrets', description: 'Update encrypted secret values and rotate keys', category: 'Secrets & Credentials' },
  { key: 'secret.delete', name: 'Delete Secrets', description: 'Delete secrets and encrypted credentials', category: 'Secrets & Credentials' },

  // Networking & Domains
  { key: 'domain.read', name: 'View Domains', description: 'View DNS hostnames and TLS status', category: 'Networking & Domains' },
  { key: 'domain.manage', name: 'Manage Domains & Routing', description: 'Assign custom domains, issue certificates, and route traffic', category: 'Networking & Domains' },

  // Integrations & Notifications
  { key: 'git.connection.read', name: 'View Git Providers', description: 'View connected GitHub, GitLab, and Bitbucket accounts', category: 'Integrations & Notifications' },
  { key: 'git.connection.manage', name: 'Manage Git Providers', description: 'Connect, update, and disconnect source control accounts', category: 'Integrations & Notifications' },
  { key: 'registry.read', name: 'View Registries', description: 'View container registries and image tags', category: 'Integrations & Notifications' },
  { key: 'registry.manage', name: 'Manage Registries', description: 'Add and configure GHCR, DockerHub, ECR, and OCI registries', category: 'Integrations & Notifications' },
  { key: 'notification.read', name: 'View Notifications', description: 'View notification channels, policies, and delivery logs', category: 'Integrations & Notifications' },
  { key: 'notification.manage', name: 'Manage Notifications', description: 'Configure Slack, Discord, Email, and webhook destinations', category: 'Integrations & Notifications' },
  { key: 'webhook.read', name: 'View Outbound Webhooks', description: 'View webhooks and event delivery attempts', category: 'Integrations & Notifications' },
  { key: 'webhook.manage', name: 'Manage Outbound Webhooks', description: 'Register endpoints, select event topics, and test webhooks', category: 'Integrations & Notifications' },

  // Observability & Compliance
  { key: 'audit.read', name: 'View Audit Logs', description: 'Inspect immutable tenant audit events and metadata', category: 'Observability & Compliance' },
]

// Authoritative role permission sets from apps/api/internal/rbac/seed.go
const DEV_OPS_KEYS = new Set([
  'organization.read', 'member.read',
  'server.read', 'server.create', 'server.update', 'server.delete',
  'project.read', 'project.create', 'project.update', 'project.delete',
  'application.read', 'application.create', 'application.update', 'application.deploy',
  'application.restart', 'application.stop', 'application.delete',
  'deployment.read', 'deployment.create', 'deployment.cancel', 'deployment.rollback',
  'git.connection.read', 'git.connection.manage',
  'registry.read', 'registry.manage',
  'domain.read', 'domain.manage',
  'secret.read_metadata', 'secret.create', 'secret.update', 'secret.delete',
  'database.read', 'database.create', 'database.update', 'database.backup', 'database.restore',
  'notification.read', 'notification.manage',
  'webhook.read', 'webhook.manage',
  'audit.read',
])

const DEVELOPER_KEYS = new Set([
  'organization.read', 'member.read', 'server.read', 'project.read',
  'application.read', 'application.create', 'application.update', 'application.deploy',
  'application.restart', 'application.stop',
  'deployment.read', 'deployment.create', 'deployment.cancel', 'deployment.rollback',
  'git.connection.read', 'git.connection.manage',
  'registry.read', 'registry.manage',
  'domain.read', 'domain.manage',
  'secret.read_metadata', 'secret.create', 'secret.update',
  'database.read', 'notification.read', 'webhook.read', 'audit.read',
])

const SUPPORT_KEYS = new Set([
  'organization.read', 'member.read', 'server.read', 'project.read',
  'application.read', 'application.restart',
  'deployment.read', 'deployment.cancel',
  'secret.read_metadata', 'database.read', 'notification.read', 'webhook.read', 'audit.read',
  'git.connection.read', 'registry.read', 'domain.read',
])

const VIEWER_KEYS = new Set([
  'organization.read', 'member.read', 'server.read', 'project.read',
  'application.read', 'deployment.read', 'secret.read_metadata', 'database.read',
  'notification.read', 'webhook.read', 'audit.read',
  'git.connection.read', 'registry.read', 'domain.read',
])

export function hasCapability(role: DefaultRoleName, capabilityKey: string): boolean {
  if (role === 'Owner') return true
  if (role === 'Administrator') return capabilityKey !== 'organization.delete'
  if (role === 'DevOps') return DEV_OPS_KEYS.has(capabilityKey)
  if (role === 'Developer') return DEVELOPER_KEYS.has(capabilityKey)
  if (role === 'Support') return SUPPORT_KEYS.has(capabilityKey)
  if (role === 'Viewer') return VIEWER_KEYS.has(capabilityKey)
  return false
}

// Sensitive key pattern to ensure secret values are NEVER displayed.
const SECRET_PATTERN =
  /secret|password|token|apikey|api_key|private|credential|bearer|auth|certificate|cert|passphrase|signature/i

const SENSITIVE_VALUE_PATTERN =
  /^(ey[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_.+/=]*|ghp_[A-Za-z0-9]{36}|glpat-[A-Za-z0-9-_]{20}|-----BEGIN[ A-Z0-9_-]+PRIVATE KEY-----)/

/** Strip or mask sensitive audit metadata so secret values NEVER render in logs or drawers. */
export function sanitizeAuditMetadata(
  metadata: Record<string, unknown> | undefined,
  action = '',
): Record<string, string> | undefined {
  if (!metadata || typeof metadata !== 'object') return undefined

  const isSecretAction = /secret|credential|token|key|password/i.test(action)

  const sanitized: Record<string, string> = {}

  for (const [key, val] of Object.entries(metadata)) {
    if (SECRET_PATTERN.test(key) || (isSecretAction && /^(value|before|after|raw|content|data)$/i.test(key))) {
      sanitized[key] = '••••••••'
      continue
    }

    if (val === null || val === undefined) {
      sanitized[key] = 'null'
      continue
    }

    if (typeof val === 'string') {
      if (SENSITIVE_VALUE_PATTERN.test(val.trim())) {
        sanitized[key] = '••••••••'
      } else {
        sanitized[key] = val
      }
      continue
    }

    if (typeof val === 'object') {
      try {
        const nestedSanitized = sanitizeAuditMetadata(val as Record<string, unknown>, action)
        sanitized[key] = JSON.stringify(nestedSanitized ?? val)
      } catch {
        sanitized[key] = '[Complex Object]'
      }
      continue
    }

    sanitized[key] = String(val)
  }

  return sanitized
}

export function sanitizeAuditEntry(entry: AuditLogEntry): AuditLogEntry {
  return {
    ...entry,
    before: sanitizeAuditMetadata(entry.before, entry.action),
    after: sanitizeAuditMetadata(entry.after, entry.action),
  }
}

export function permissionLevelLabel(level: PermissionLevel): string {
  return level
}

// ---------------------------------------------------------------------------
// Wire Types and Control Plane Client Integrations
// ---------------------------------------------------------------------------

export interface WireRole {
  key: string
  name: string
  description: string
}

export interface WireMember {
  id: string
  userId: string
  email: string
  displayName: string
  status: string
  roles: WireRole[]
  joinedAt?: string
  createdAt: string
}

export interface WireInvitation {
  id: string
  email: string
  roles: WireRole[]
  expiresAt: string
  createdAt: string
  token?: string
  status?: string
  invitedBy?: string
  acceptedAt?: string
  revokedAt?: string
}

export interface WireAuditRecord {
  id: string
  organizationId?: string
  actorUserId?: string
  actorType: string
  action: string
  resourceType: string
  resourceId?: string
  requestId?: string
  ipAddress?: string
  userAgent?: string
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  createdAt: string
}

export async function fetchOrganizationMembers(orgId: string): Promise<Page<WireMember>> {
  return apiClient.get<Page<WireMember>>(`/organizations/${orgId}/members`, { orgId })
}

export async function updateMemberRoles(
  orgId: string,
  memberId: string,
  roleKeys: string[],
): Promise<{ member: WireMember }> {
  return apiClient.patch<{ member: WireMember }>(
    `/organizations/${orgId}/members/${memberId}`,
    { roleKeys },
    { orgId },
  )
}

export async function removeMember(orgId: string, memberId: string): Promise<{ ok: boolean }> {
  return apiClient.delete<{ ok: boolean }>(`/organizations/${orgId}/members/${memberId}`, { orgId })
}

export async function fetchOrganizationInvitations(orgId: string): Promise<Page<WireInvitation>> {
  return apiClient.get<Page<WireInvitation>>(`/organizations/${orgId}/invitations`, { orgId })
}

export async function createOrganizationInvitation(
  orgId: string,
  email: string,
  roleKeys: string[],
): Promise<{ invitation: WireInvitation }> {
  return apiClient.post<{ invitation: WireInvitation }>(
    `/organizations/${orgId}/invitations`,
    { email, roleKeys },
    { orgId },
  )
}

export async function revokeOrganizationInvitation(
  orgId: string,
  invitationId: string,
): Promise<{ ok: boolean }> {
  return apiClient.delete<{ ok: boolean }>(
    `/organizations/${orgId}/invitations/${invitationId}`,
    { orgId },
  )
}

export async function fetchAuditLogs(
  orgId: string,
  params?: {
    action?: string
    resourceType?: string
    resourceId?: string
    actorUserId?: string
    limit?: number
    offset?: number
  },
): Promise<Page<WireAuditRecord>> {
  const q = new URLSearchParams()
  q.set('organizationId', orgId)
  if (params?.action) q.set('action', params.action)
  if (params?.resourceType) q.set('resourceType', params.resourceType)
  if (params?.resourceId) q.set('resourceId', params.resourceId)
  if (params?.actorUserId) q.set('actorUserId', params.actorUserId)
  if (params?.limit) q.set('limit', String(params.limit))
  if (params?.offset) q.set('offset', String(params.offset))

  return apiClient.get<Page<WireAuditRecord>>(`/audit-logs?${q.toString()}`, { orgId })
}

// Mappers from Wire models to UI View models
export function mapWireMemberToTeamMember(wire: WireMember): TeamMember {
  const primaryRole = wire.roles?.[0]?.name || 'Viewer'
  const roleName = DEFAULT_ROLES.find((r) => r.name.toLowerCase() === primaryRole.toLowerCase())?.name || 'Viewer'

  return {
    id: wire.id,
    name: wire.displayName || wire.email.split('@')[0],
    email: wire.email,
    role: roleName,
    teams: ['Organization'],
    status: wire.status === 'active' ? 'healthy' : wire.status === 'invited' ? 'pending' : 'degraded',
    lastActive: wire.joinedAt ? new Date(wire.joinedAt).toLocaleDateString() : 'Pending invite',
  }
}

export function mapWireInvitationToInvitation(wire: WireInvitation): Invitation {
  const roleName = wire.roles?.[0]?.name || 'Viewer'
  let status: Invitation['status'] = 'pending'
  if (wire.status === 'accepted') status = 'accepted'
  else if (wire.status === 'revoked') status = 'revoked'
  else if (wire.status === 'expired') status = 'expired'
  else {
    const expires = new Date(wire.expiresAt).getTime()
    if (expires < Date.now()) status = 'expired'
  }

  return {
    id: wire.id,
    email: wire.email,
    role: roleName,
    teams: ['Organization'],
    invitedBy: wire.invitedBy || 'Workspace Administrator',
    invitedAt: new Date(wire.createdAt).toLocaleDateString(),
    expiresAt: new Date(wire.expiresAt).toLocaleDateString(),
    status,
  }
}

export function mapWireAuditRecordToEntry(rec: WireAuditRecord): AuditLogEntry {
  // Never expose secret values
  const before = sanitizeAuditMetadata(rec.before, rec.action)
  const after = sanitizeAuditMetadata(rec.after, rec.action)

  return {
    id: rec.id,
    timestamp: new Date(rec.createdAt).toISOString().replace('T', ' ').slice(0, 19) + ' UTC',
    actor: rec.actorUserId || rec.actorType || 'system',
    action: rec.action,
    resource: rec.resourceId ? `${rec.resourceType} (${rec.resourceId.slice(0, 8)})` : rec.resourceType,
    project: rec.organizationId ? `org-${rec.organizationId.slice(0, 8)}` : 'Global',
    ip: rec.ipAddress || '127.0.0.1',
    result: 'success',
    before,
    after,
  }
}
