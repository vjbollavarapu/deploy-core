/**
 * DeployCore control-plane API contract types (B32).
 *
 * Sourced from apps/api/docs/openapi.json — wire-format shapes only.
 * Do not couple UI view-models in lib/types.ts to DB rows or Go structs.
 * Prefer mapping API → UI types at the client boundary.
 */

export const API_BASE = '/api/v1' as const

/** Wire-format UUID string (not a branded runtime type). */
export type UUID = string

/** RFC3339 timestamp string. */
export type Timestamp = string

export type JSONObject = Record<string, unknown>

export type ErrorCode = "VALIDATION_ERROR" | "UNAUTHORIZED" | "FORBIDDEN" | "RESOURCE_NOT_FOUND" | "CONFLICT" | "INTERNAL_ERROR" | "SERVICE_UNAVAILABLE" | "RATE_LIMITED" | "ORGANIZATION_NOT_FOUND" | "PROJECT_NOT_FOUND" | "ENVIRONMENT_NOT_FOUND" | "SERVER_NOT_FOUND" | "SERVER_OFFLINE" | "AGENT_UNAVAILABLE" | "APPLICATION_NOT_FOUND" | "DEPLOYMENT_NOT_FOUND" | "REVISION_NOT_FOUND" | "REVISION_NOT_READY" | "DOMAIN_ALREADY_ASSIGNED" | "SECRET_NOT_FOUND" | "INSUFFICIENT_RESOURCES" | "BUILD_TIMEOUT" | "HEALTH_CHECK_FAILED" | "DATABASE_NOT_FOUND" | "VOLUME_NOT_FOUND" | "VOLUME_IN_USE"

export type UserStatus = "active" | "disabled" | "pending"

export type ServerStatus = "ONLINE" | "DEGRADED" | "OFFLINE" | "MAINTENANCE" | "DISABLED"

export type ApplicationType = "WEB_SERVICE" | "API" | "WORKER" | "SCHEDULED_JOB" | "STATIC_SITE" | "DOCKER_COMPOSE" | "DOCKER_IMAGE"

export type SourceType = "git" | "image" | "compose" | "upload"

export type RestartPolicy = "always" | "unless-stopped" | "on-failure" | "no"

export type DeploymentStatus = "PENDING" | "QUEUED" | "PREPARING" | "FETCHING_SOURCE" | "BUILDING" | "IMAGE_READY" | "CREATING_CONTAINER" | "STARTING" | "HEALTH_CHECKING" | "ACTIVATING" | "RUNNING" | "SOURCE_FAILED" | "BUILD_FAILED" | "IMAGE_FAILED" | "CONTAINER_FAILED" | "START_FAILED" | "HEALTH_CHECK_FAILED" | "ROUTING_FAILED" | "CANCELLED" | "TIMEOUT"

export type DeploymentTrigger = "manual" | "git_push" | "api" | "rollback" | "schedule" | "system"

export type RevisionStatus = "CREATED" | "READY" | "ACTIVE" | "INACTIVE" | "FAILED" | "ARCHIVED"

export type ReplicaStatus = "PENDING" | "STARTING" | "RUNNING" | "UNHEALTHY" | "STOPPING" | "STOPPED" | "FAILED"

export type VolumeState = "PENDING" | "CREATING" | "READY" | "ATTACHED" | "DETACHING" | "DELETING" | "FAILED" | "DELETED"

export type DatabaseStatus = "PENDING" | "PROVISIONING" | "RUNNING" | "STOPPED" | "DEGRADED" | "FAILED" | "DELETING" | "DELETED"

export type DNSStatus = "PENDING" | "VALID" | "INVALID"

export type TLSStatus = "PENDING" | "ISSUING" | "ACTIVE" | "EXPIRING" | "FAILED"

export type HealthStatus = "UNKNOWN" | "STARTING" | "HEALTHY" | "DEGRADED" | "UNHEALTHY"

export type ConfigScope = "ORGANIZATION" | "PROJECT" | "ENVIRONMENT" | "APPLICATION"

export type EnvironmentKind = "production" | "staging" | "development" | "preview" | "custom"

export type Permission = "organization.read" | "organization.update" | "organization.delete" | "member.read" | "member.invite" | "member.update" | "member.remove" | "server.read" | "server.create" | "server.update" | "server.delete" | "project.read" | "project.create" | "project.update" | "project.delete" | "application.read" | "application.create" | "application.update" | "application.deploy" | "application.restart" | "application.stop" | "application.delete" | "deployment.read" | "deployment.create" | "deployment.cancel" | "deployment.rollback" | "git.connection.read" | "git.connection.manage" | "registry.read" | "registry.manage" | "domain.read" | "domain.manage" | "secret.read_metadata" | "secret.create" | "secret.update" | "secret.delete" | "database.read" | "database.create" | "database.update" | "database.backup" | "database.restore" | "notification.read" | "notification.manage" | "webhook.read" | "webhook.manage" | "audit.read"

export type SystemRole = "owner" | "administrator" | "devops" | "developer" | "support" | "viewer"

export interface APIError {
  code: ErrorCode
  message: string
  requestId?: string
  details?: Record<string, unknown>
}

export interface ErrorEnvelope {
  error: APIError
}

export interface TokenPair {
  accessToken?: string
  refreshToken?: string
  accessExpiresAt?: Timestamp
  refreshExpiresAt?: Timestamp
  tokenType?: string
}

export interface User {
  id?: UUID
  email?: string
  displayName?: string
  status?: UserStatus
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface AuthResult {
  user?: User
  tokens?: TokenPair
  mfa?: {
    required?: boolean
    methods?: Array<string>
  }
}

export interface RegisterRequest {
  email: string
  password: string
  displayName?: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface RefreshRequest {
  refreshToken: string
}

export interface LogoutRequest {
  refreshToken?: string
}

export interface CreateProjectRequest {
  organizationId: UUID
  name: string
  slug: string
  description?: string
}

export interface CreateServerRequest {
  organizationId: UUID
  name: string
  provider?: string
  hostname?: string
  region?: string
  labels?: Record<string, string>
  cpuCores?: number
  memoryBytes?: number
  diskBytes?: number
}

export interface ApplicationConfigInput {
  sourceType?: SourceType
  repositoryUrl?: string | null
  gitBranch?: string | null
  dockerfilePath?: string | null
  buildContext?: string | null
  imageReference?: string | null
  internalPort?: number | null
  command?: string | null
  entrypoint?: string | null
  cpuLimitMillis?: number | null
  memoryLimitBytes?: number | null
  restartPolicy?: RestartPolicy
  healthCheck?: Record<string, unknown>
  runtimeConfig?: Record<string, unknown>
  autoDeployEnabled?: boolean
  gitConnectionId?: string | null
  desiredReplicas?: number
}

export interface CreateApplicationRequest {
  organizationId: UUID
  projectId: UUID
  environmentId: UUID
  name: string
  slug: string
  type: ApplicationType
  targetServerId?: string | null
  placementPolicy?: Record<string, unknown>
  config: ApplicationConfigInput
}

export interface CreateDeploymentRequest {
  trigger?: DeploymentTrigger
  idempotencyKey?: string
  metadata?: Record<string, unknown>
}

export interface RollbackRequest {
  targetRevisionId: UUID
}

export interface ScaleReplicasRequest {
  desiredReplicas: number
}

export interface Organization {
  id?: UUID
  name?: string
  slug?: string
  status?: string
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface Server {
  id?: UUID
  organizationId?: UUID
  name?: string
  status?: ServerStatus
  maintenanceMode?: boolean
  hostname?: string
  provider?: string
  labels?: Record<string, string>
  lastHeartbeatAt?: string | null
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface Application {
  id?: UUID
  organizationId?: UUID
  projectId?: UUID
  environmentId?: UUID
  name?: string
  slug?: string
  type?: ApplicationType
  status?: string
  targetServerId?: string | null
  placementPolicy?: Record<string, unknown>
  config?: ApplicationConfigInput
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface Deployment {
  id?: UUID
  organizationId?: UUID
  applicationId?: UUID
  revisionId?: string | null
  status?: DeploymentStatus
  trigger?: DeploymentTrigger
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface PaginatedMeta {
  items?: Array<JSONObject>
  limit?: number
  offset?: number
  totalCount?: number | null
}

/** Offset pagination query used by list endpoints. */
export interface PageParams {
  limit?: number
  offset?: number
}

export interface Page<T> {
  items: T[]
  limit: number
  offset: number
  totalCount?: number | null
}

/** Permission keys enforced by the control plane (org-scoped). */
export type PermissionKey = Permission

export interface Project {
  id?: UUID
  organizationId?: UUID
  name?: string
  slug?: string
  description?: string
  createdBy?: UUID
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface UpdateProjectRequest {
  name?: string
  slug?: string
  description?: string
}

export interface Environment {
  id?: UUID
  organizationId?: UUID
  projectId?: UUID
  name?: string
  slug?: string
  kind?: EnvironmentKind
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface CreateEnvironmentRequest {
  name: string
  slug: string
  kind: EnvironmentKind
}

export interface UpdateEnvironmentRequest {
  name?: string
  slug?: string
  kind?: EnvironmentKind
}

export interface Database {
  id?: UUID
  organizationId?: UUID
  projectId?: UUID
  environmentId?: UUID
  serverId?: UUID
  name?: string
  engine?: string
  engineVersion?: string
  databaseName?: string
  username?: string
  storageVolumeName?: string
  volumeProtected?: boolean
  cpuMillis?: number | null
  memoryBytes?: number | null
  containerRuntimeId?: string | null
  status?: DatabaseStatus
  backupPolicy?: Record<string, unknown>
  provisionCommandId?: string | null
  lastError?: string
  hasCredential?: boolean
  createdBy?: UUID
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface CreateDatabaseRequest {
  organizationId?: UUID
  projectId?: UUID
  environmentId: UUID
  serverId: UUID
  name: string
  engine: string
  engineVersion?: string
  databaseName?: string
  username?: string
  password?: string
  storageVolume?: string
  cpuMillis?: number | null
  memoryBytes?: number | null
  backupPolicy?: Record<string, unknown>
}

export interface UpdateDatabaseRequest {
  name?: string
  cpuMillis?: number | null
  memoryBytes?: number | null
  backupPolicy?: Record<string, unknown>
  status?: DatabaseStatus
  rotatePassword?: string
}

export interface DomainRoutingConfig {
  healthCheckPath?: string
  stripPrefix?: boolean
  customHeaders?: Record<string, string>
}

export interface Domain {
  id?: UUID
  organizationId?: UUID
  applicationId?: UUID
  environmentId?: UUID
  hostname?: string
  internalPort?: number
  isPrimary?: boolean
  forceHttps?: boolean
  dnsStatus?: DNSStatus
  tlsStatus?: TLSStatus
  routing?: DomainRoutingConfig
  createdAt?: Timestamp
  updatedAt?: Timestamp
}

export interface CreateDomainRequest {
  hostname: string
  internalPort?: number
  isPrimary?: boolean
  forceHttps?: boolean
}

export interface UpdateDomainRequest {
  hostname?: string
  internalPort?: number
  isPrimary?: boolean
  forceHttps?: boolean
  dnsStatus?: DNSStatus
  tlsStatus?: TLSStatus
}

