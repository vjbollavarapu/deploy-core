export type Status =
  | 'healthy'
  | 'running'
  | 'deploying'
  | 'pending'
  | 'queued'
  | 'stopped'
  | 'degraded'
  | 'failed'
  | 'offline'
  | 'maintenance'
  | 'cancelled'
  | 'unknown'

export type StatusTone = 'success' | 'warning' | 'critical' | 'info' | 'inactive'

export interface Project {
  id: string
  name: string
  slug: string
  description?: string
  environments: string[]
  applicationCount: number
  health: Status
  lastDeployment: string
  owner: { name: string; avatar?: string }
  updatedAt: string
}

export type RuntimeType =
  | 'Web Service'
  | 'API'
  | 'Worker'
  | 'Scheduled Job'
  | 'Static Site'
  | 'Docker Compose'
  | 'Docker Image'

export interface Application {
  id: string
  name: string
  projectId: string
  project: string
  environment: string
  runtime: RuntimeType
  server: string
  revision: string
  status: Status
  domain: string
  lastDeployment: string
  repo: string
  branch: string
  commit: string
  commitMessage: string
  cpu: number
  cpuLimit: number
  memory: number
  memoryLimit: number
  instances: number
  uptime: string
}

export interface DeploymentStep {
  phase: DeploymentPhase
  name: string
  status: 'complete' | 'active' | 'pending' | 'failed'
  failureReason?: DeploymentFailureReason
}

export type DeploymentPhase =
  | 'PENDING'
  | 'QUEUED'
  | 'PREPARING'
  | 'FETCHING_SOURCE'
  | 'BUILDING'
  | 'IMAGE_READY'
  | 'CREATING_CONTAINER'
  | 'STARTING'
  | 'HEALTH_CHECKING'
  | 'ACTIVATING'
  | 'RUNNING'

export type DeploymentFailureReason =
  | 'SOURCE_FAILED'
  | 'BUILD_FAILED'
  | 'IMAGE_FAILED'
  | 'CONTAINER_FAILED'
  | 'START_FAILED'
  | 'HEALTH_CHECK_FAILED'
  | 'ROUTING_FAILED'
  | 'CANCELLED'
  | 'TIMEOUT'

export interface DeploymentEvent {
  id: string
  timestamp: string
  phase: DeploymentPhase | DeploymentFailureReason
  message: string
  tone: StatusTone
}

export interface Deployment {
  id: string
  number: number
  applicationId: string
  application: string
  project: string
  environment: string
  revision: string
  commit: string
  commitMessage: string
  author: { name: string; avatar?: string }
  triggeredBy: string
  status: Status
  phase: DeploymentPhase
  failureReason?: DeploymentFailureReason
  duration: string
  startedAt: string
  repo: string
  branch: string
  server: string
  image: string
  steps: DeploymentStep[]
  events: DeploymentEvent[]
}

export interface Server {
  id: string
  name: string
  provider: string
  /** Inventory region from Control Plane; null when not reported. */
  region: string | null
  /** Public IP from Control Plane; null when not reported. */
  ip: string | null
  privateIp: string | null
  /** Live utilization % — only when Control Plane/agent reports it. */
  cpu: number | null
  cpuCores: number | null
  memory: number | null
  memoryTotalGb: number | null
  disk: number | null
  diskTotalGb: number | null
  containers: number | null
  agentVersion: string | null
  status: Status
  /** Human-readable relative heartbeat; null when never heartbeated. */
  lastHeartbeat: string | null
  /** Raw ISO timestamp from API; null means no successful agent heartbeat yet. */
  lastHeartbeatAt: string | null
  os: string | null
  arch: string | null
  dockerVersion: string | null
  uptime: string | null
  load: [number, number, number] | null
}

export interface DatabaseInstance {
  id: string
  name: string
  type: 'PostgreSQL' | 'MySQL' | 'Redis' | 'MongoDB'
  version: string
  project: string
  environment: string
  server: string
  storageUsedGb: number
  storageTotalGb: number
  backups: number
  lastBackup: string
  status: Status
  /** Logical database name inside the engine. */
  dbName: string
  port: number
  username: string
  connectionHost: string
  /**
   * API policy flag: when false, the UI must never reveal connection credentials
   * even if the operator claims permission.
   */
  credentialsRevealAllowed: boolean
}

export interface ActivityItem {
  id: string
  actor: string
  action: string
  target: string
  timestamp: string
  tone: StatusTone
}

export interface AuditLogEntry {
  id: string
  timestamp: string
  actor: string
  action: string
  resource: string
  project: string
  ip: string
  result: 'success' | 'failure'
  before?: Record<string, string>
  after?: Record<string, string>
}

export interface SecretItem {
  id: string
  name: string
  scope: string
  applications: string[]
  updatedAt: string
  updatedBy: string
  /** Metadata only — secret values are never returned from the server. */
  lastRotatedAt: string
  accessRoles: string[]
  description?: string
}

export interface LogLine {
  id: string
  timestamp: string
  level: 'info' | 'warn' | 'error' | 'debug'
  container: string
  message: string
  /** Optional correlators for fleet / filtered log explorers. */
  application?: string
  environment?: string
  revision?: string
}

export interface PlatformEvent {
  id: string
  timestamp: string
  category: 'deployment' | 'infrastructure' | 'security' | 'backup' | 'certificate' | 'incident'
  actor: string
  action: string
  target: string
  tone: StatusTone
  project?: string
  environment?: string
}

/** Environment variable metadata on a revision snapshot — never includes secret values. */
export interface RevisionEnvVarMeta {
  key: string
  scope: 'Organization' | 'Project' | 'Environment' | 'Application'
  secret: boolean
}

export interface Revision {
  id: string
  number: string
  applicationId: string
  application: string
  environment: string
  status: Status
  traffic: number
  commit: string
  commitMessage: string
  image: string
  imageDigest: string
  createdBy: { name: string; avatar?: string }
  createdAt: string
  runtime: RuntimeType
  server: string
  command: string
  cpuLimit: number
  memoryLimit: number
  /** Derived length of envVars; kept for compact table display. */
  envVarCount: number
  /** Keys and scopes only — values are never stored on revisions. */
  envVars: RevisionEnvVarMeta[]
  /** Secret names referenced by this revision (contents never shown). */
  secretRefs: string[]
  domains: string[]
  volumes: string[]
  healthCheck: { type: string; path: string; interval: string }
  archived?: boolean
}

export interface DnsRecord {
  type: string
  name: string
  value: string
}

export interface RedirectRule {
  from: string
  to: string
  code: 301 | 302 | 307 | 308
}

/** TLS / domain provisioning lifecycle (F10). */
export type DomainTlsState =
  | 'PENDING'
  | 'VERIFYING'
  | 'ISSUING'
  | 'ACTIVE'
  | 'EXPIRING'
  | 'FAILED'

export interface DomainRecord {
  id: string
  domain: string
  applicationId: string
  application: string
  environment: string
  routingPort: number
  dnsVerified: boolean
  https: boolean
  certExpiry: string
  certExpiryDays: number
  /** Operational health badge (host/routing). */
  status: Status
  /** Certificate / DNS provisioning lifecycle. */
  tlsState: DomainTlsState
  certificateIssuer: string | null
  certificateIssuedAt: string | null
  nextRenewalAt: string | null
  lastValidatedAt: string | null
  validationMessage: string
  primary: boolean
  forceHttps: boolean
  requiredRecord: DnsRecord
  detectedRecord: DnsRecord | null
  redirectRules: RedirectRule[]
}

export interface Volume {
  id: string
  name: string
  server: string
  driver: string
  attachedResource: string
  mountPath: string
  backupPolicy: string
  status: Status
  usedGb: number
  totalGb: number
  mounts: number
}

export interface DockerNetwork {
  id: string
  name: string
  driver: string
  scope: string
  project: string
  environment: string
  connectedServices: string[]
}

export interface Container {
  id: string
  name: string
  applicationId: string
  application: string
  revision: string
  server: string
  image: string
  cpu: number
  memory: number
  memoryLimitMb: number
  restarts: number
  status: Status
}

export interface ContainerImage {
  id: string
  name: string
  tag: string
  digest: string
  application: string
  sizeMb: number
  createdAt: string
  location: string
}

export type RegistryType =
  | 'GitHub Container Registry'
  | 'Docker Hub'
  | 'AWS ECR'
  | 'GCP Artifact Registry'
  | 'Azure Container Registry'
  | 'Generic OCI Registry'

export interface Registry {
  id: string
  type: RegistryType
  name: string
  url: string
  status: Status
  connectedAt: string
  imageCount: number
}

export type GitProviderType = 'GitHub' | 'GitLab' | 'Bitbucket' | 'Generic Git'

export interface GitProviderConnection {
  id: string
  type: GitProviderType
  account: string
  organizations: string[]
  repositoryCount: number
  status: Status
  permissions: string[]
  lastSync: string
}

export interface EnvVarEntry {
  id: string
  key: string
  value: string
  secret: boolean
  scope: 'Organization' | 'Project' | 'Environment' | 'Application'
  source: string
  overridden: boolean
}

export interface HealthCheckHistoryEntry {
  timestamp: string
  status: 'pass' | 'fail'
  latencyMs: number
}

export type HealthCheckType = 'HTTP' | 'TCP' | 'Command' | 'Container Status'

export interface HealthCheckConfig {
  id: string
  applicationId: string
  application: string
  type: HealthCheckType
  path: string
  port: number
  interval: number
  timeout: number
  retries: number
  initialDelay: number
  expectedStatus: number
  status: Status
  history: HealthCheckHistoryEntry[]
}

export interface BackupJob {
  id: string
  database: string
  databaseId: string
  policy: string
  destination: string
  lastSuccess: string
  nextRun: string
  retention: string
  status: Status
}

export interface BackupRun {
  id: string
  backupJobId: string
  database: string
  databaseId: string
  status: 'success' | 'failed' | 'running' | 'queued'
  startedAt: string
  completedAt: string
  duration: string
  sizeGb: number
  destination: string
  checksum: string
}

export type NotificationChannelType =
  | 'Email'
  | 'Slack'
  | 'Microsoft Teams'
  | 'Discord'
  | 'Telegram'
  | 'Webhook'
  | 'WhatsApp'

export interface NotificationChannel {
  id: string
  type: NotificationChannelType
  name: string
  target: string
  status: Status
}

export interface NotificationPolicy {
  id: string
  name: string
  when: string
  conditions: string[]
  channels: string[]
  enabled: boolean
}

export interface WebhookDelivery {
  id: string
  timestamp: string
  statusCode: number
  latencyMs: number
  request: string
  response: string
  retried: boolean
  /** Total delivery attempts including the initial try. */
  attempts: number
}

export interface Webhook {
  id: string
  name: string
  endpoint: string
  events: string[]
  enabled: boolean
  status: Status
  latestDelivery: string
  /** Masked signing secret — never return the raw secret from the API. */
  signingSecretMasked: string
  deliveries: WebhookDelivery[]
}

export interface TeamMember {
  id: string
  name: string
  email: string
  role: string
  teams: string[]
  status: Status
  lastActive: string
}

export interface Team {
  id: string
  name: string
  memberCount: number
  members: string[]
}

export type InvitationStatus = 'pending' | 'accepted' | 'expired' | 'revoked'

export interface Invitation {
  id: string
  email: string
  role: string
  teams: string[]
  invitedBy: string
  invitedAt: string
  expiresAt: string
  status: InvitationStatus
}

export type PermissionLevel = 'full' | 'edit' | 'view' | 'none'

export interface Organization {
  id: string
  name: string
  plan: string
  userCount: number
  serverCount: number
  status: Status
  createdAt: string
}

export interface PlatformJob {
  id: string
  name: string
  type: string
  status: Status
  startedAt: string
  duration: string
}

export interface FeatureFlag {
  id: string
  name: string
  key: string
  enabled: boolean
  rollout: number
  environment: string
}
