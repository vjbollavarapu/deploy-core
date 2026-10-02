/** Wire-to-view reads for production application and deployment detail. */

export interface DetailClient {
  get<T>(path: string): Promise<T>
}

export type LoadResult<T> =
  | { kind: 'ok'; value: T }
  | { kind: 'not-found' }
  | { kind: 'error'; message: string }

export interface WireApplicationConfig {
  sourceType?: string
  repositoryUrl?: string | null
  gitBranch?: string | null
  imageReference?: string | null
  cpuLimitMillis?: number | null
  memoryLimitBytes?: number | null
  runtimeConfig?: Record<string, unknown>
}

export interface WireApplicationDetail {
  id?: string
  organizationId?: string
  projectId?: string
  environmentId?: string
  name?: string
  slug?: string
  type?: string
  status?: string
  targetServerId?: string | null
  config?: WireApplicationConfig
}

export interface ApplicationDetail {
  id: string
  organizationId: string | null
  name: string
  slug: string | null
  type: string
  status: string
  projectId: string | null
  projectName: string | null
  environmentId: string | null
  environmentName: string | null
  serverId: string | null
  serverName: string | null
  repositoryUrl: string | null
  gitBranch: string | null
  imageReference: string | null
  cpuLimitMillis: number | null
  memoryLimitBytes: number | null
  desiredReplicas: number | null
}

export interface WireDeploymentEvent {
  id?: string
  fromStatus?: string | null
  toStatus?: string
  message?: string
  createdAt?: string
}

export interface WireDeploymentDetail {
  id?: string
  organizationId?: string
  applicationId?: string
  environmentId?: string
  serverId?: string | null
  status?: string
  trigger?: string
  errorCode?: string | null
  errorMessage?: string | null
  activeRevisionId?: string | null
  targetRevisionId?: string | null
  startedAt?: string | null
  finishedAt?: string | null
  createdAt?: string
  updatedAt?: string
  events?: WireDeploymentEvent[]
}

export interface DeploymentEventDetail {
  id: string
  fromStatus: string | null
  toStatus: string
  message: string
  createdAt: string | null
}

export interface DeploymentDetail {
  id: string
  applicationId: string | null
  applicationName: string | null
  environmentId: string | null
  environmentName: string | null
  serverId: string | null
  serverName: string | null
  status: string
  trigger: string | null
  errorCode: string | null
  errorMessage: string | null
  activeRevisionId: string | null
  targetRevisionId: string | null
  startedAt: string | null
  finishedAt: string | null
  createdAt: string | null
  duration: string | null
  events: DeploymentEventDetail[]
}

export interface VariableDetail {
  id: string
  key: string
  value: string
  scope: string
}

const SYNTHETIC_MARKERS = [
  'rev-init',
  'deploycore.app',
  'ghcr.io/deploycore',
  'a1b2c3d',
  'Core Platform',
  'srv-primary',
  'Just now',
  'Manual Trigger',
]

export function syntheticMarkers(value: unknown): string[] {
  const text = JSON.stringify(value)
  return SYNTHETIC_MARKERS.filter((marker) => text.includes(marker))
}

export function applicationDeploymentsPath(organizationId: string, applicationId: string): string {
  return `/deployments?organizationId=${encodeURIComponent(organizationId)}&applicationId=${encodeURIComponent(applicationId)}`
}

export function applicationVariablesPath(organizationId: string, applicationId: string): string {
  return `/variables?organizationId=${encodeURIComponent(organizationId)}&applicationId=${encodeURIComponent(applicationId)}`
}

export function applicationLogsPath(applicationId: string): string {
  return `/applications/${encodeURIComponent(applicationId)}/logs?follow=false`
}

export function applicationSectionMode(section: string, demo: boolean): 'volumes' | 'production' | 'demo' {
  if (section === 'volumes') return 'volumes'
  if (!demo) return 'production'
  return 'demo'
}

export function toSettingsApplication(application: ApplicationDetail) {
  const show = (value: string | null) => (value && value.trim() ? value : '—')
  return {
    id: application.id,
    name: application.name,
    runtime: application.type,
    project: show(application.projectName ?? application.projectId),
    environment: show(application.environmentName ?? application.environmentId),
    server: show(application.serverName ?? application.serverId),
    instances: application.desiredReplicas == null ? '—' : String(application.desiredReplicas),
    repo: show(application.repositoryUrl ?? application.imageReference),
    branch: show(application.gitBranch),
  }
}

export function mapApplicationStatus(raw?: string): string {
  switch ((raw ?? '').toLowerCase()) {
    case 'draft':
      return 'pending'
    case 'ready':
      return 'pending'
    case 'deploying':
      return 'deploying'
    case 'running':
      return 'running'
    case 'stopped':
      return 'stopped'
    case 'failed':
      return 'failed'
    case 'archived':
      return 'stopped'
    default:
      return raw && raw.trim() ? raw : 'unknown'
  }
}

export function mapRuntimeLabel(raw?: string): string {
  switch (raw) {
    case 'API':
      return 'API'
    case 'WORKER':
      return 'Worker'
    case 'SCHEDULED_JOB':
      return 'Scheduled Job'
    case 'STATIC_SITE':
      return 'Static Site'
    case 'DOCKER_COMPOSE':
      return 'Docker Compose'
    case 'DOCKER_IMAGE':
      return 'Docker Image'
    case 'WEB_SERVICE':
      return 'Web Service'
    default:
      return raw && raw.trim() ? raw : 'Unknown'
  }
}

function textOrNull(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

function numberOrNull(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return null
}

function desiredReplicas(config?: WireApplicationConfig): number | null {
  const raw = config?.runtimeConfig?.desiredReplicas
  const parsed = numberOrNull(raw)
  if (parsed == null || parsed < 1) return null
  return parsed
}

export function mapWireApplication(
  wire: WireApplicationDetail,
  names?: {
    projectName?: string | null
    environmentName?: string | null
    serverName?: string | null
  },
): ApplicationDetail {
  return {
    id: wire.id ?? '',
    organizationId: textOrNull(wire.organizationId),
    name: textOrNull(wire.name) ?? 'Untitled',
    slug: textOrNull(wire.slug),
    type: mapRuntimeLabel(wire.type),
    status: mapApplicationStatus(wire.status),
    projectId: textOrNull(wire.projectId),
    projectName: textOrNull(names?.projectName),
    environmentId: textOrNull(wire.environmentId),
    environmentName: textOrNull(names?.environmentName),
    serverId: textOrNull(wire.targetServerId),
    serverName: textOrNull(names?.serverName),
    repositoryUrl: textOrNull(wire.config?.repositoryUrl),
    gitBranch: textOrNull(wire.config?.gitBranch),
    imageReference: textOrNull(wire.config?.imageReference),
    cpuLimitMillis: numberOrNull(wire.config?.cpuLimitMillis),
    memoryLimitBytes: numberOrNull(wire.config?.memoryLimitBytes),
    desiredReplicas: desiredReplicas(wire.config),
  }
}

function durationBetween(startedAt: string | null, finishedAt: string | null): string | null {
  if (!startedAt || !finishedAt) return null
  const start = Date.parse(startedAt)
  const end = Date.parse(finishedAt)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return null
  const seconds = Math.round((end - start) / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return `${minutes}m ${rest}s`
}

export function mapWireDeployment(
  wire: WireDeploymentDetail,
  names?: { applicationName?: string | null; environmentName?: string | null; serverName?: string | null },
): DeploymentDetail {
  const startedAt = textOrNull(wire.startedAt)
  const finishedAt = textOrNull(wire.finishedAt)
  const events = (wire.events ?? []).map((event, index) => ({
    id: textOrNull(event.id) ?? `event-${index}`,
    fromStatus: textOrNull(event.fromStatus),
    toStatus: textOrNull(event.toStatus) ?? 'unknown',
    message: textOrNull(event.message) ?? '',
    createdAt: textOrNull(event.createdAt),
  }))
  return {
    id: wire.id ?? '',
    applicationId: textOrNull(wire.applicationId),
    applicationName: textOrNull(names?.applicationName),
    environmentId: textOrNull(wire.environmentId),
    environmentName: textOrNull(names?.environmentName),
    serverId: textOrNull(wire.serverId),
    serverName: textOrNull(names?.serverName),
    status: textOrNull(wire.status) ?? 'unknown',
    trigger: textOrNull(wire.trigger),
    errorCode: textOrNull(wire.errorCode),
    errorMessage: textOrNull(wire.errorMessage),
    activeRevisionId: textOrNull(wire.activeRevisionId),
    targetRevisionId: textOrNull(wire.targetRevisionId),
    startedAt,
    finishedAt,
    createdAt: textOrNull(wire.createdAt),
    duration: durationBetween(startedAt, finishedAt),
    events,
  }
}

export function mapWireVariable(wire: {
  id?: string
  key?: string
  value?: string
  scope?: string
}): VariableDetail {
  return {
    id: textOrNull(wire.id) ?? '',
    key: textOrNull(wire.key) ?? '',
    value: typeof wire.value === 'string' ? wire.value : '',
    scope: textOrNull(wire.scope) ?? 'UNKNOWN',
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) return err.message
  return 'Unable to reach the control plane'
}

function isNotFound(err: unknown): boolean {
  return typeof err === 'object' && err !== null && 'status' in err && (err as { status?: number }).status === 404
}

async function readName(
  client: DetailClient,
  path: string,
  pick: (body: Record<string, unknown>) => unknown,
): Promise<string | null> {
  try {
    const body = await client.get<Record<string, unknown>>(path)
    return textOrNull(pick(body))
  } catch {
    return null
  }
}

export async function loadProductionApplication(
  client: DetailClient,
  applicationId: string,
): Promise<LoadResult<ApplicationDetail>> {
  let wire: WireApplicationDetail
  try {
    const body = await client.get<{ application?: WireApplicationDetail }>(`/applications/${applicationId}`)
    if (!body.application?.id) return { kind: 'not-found' }
    wire = body.application
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }

  const [projectName, environmentName, serverName] = await Promise.all([
    wire.projectId ? readName(client, `/projects/${wire.projectId}`, (body) => (body.project as { name?: string } | undefined)?.name) : Promise.resolve(null),
    wire.environmentId
      ? readName(client, `/environments/${wire.environmentId}`, (body) => (body.environment as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
    wire.targetServerId
      ? readName(client, `/servers/${wire.targetServerId}`, (body) => (body.server as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
  ])

  return {
    kind: 'ok',
    value: mapWireApplication(wire, { projectName, environmentName, serverName }),
  }
}

export async function loadProductionApplicationDeployments(
  client: DetailClient,
  organizationId: string,
  applicationId: string,
): Promise<LoadResult<DeploymentDetail[]>> {
  try {
    const body = await client.get<{ items?: WireDeploymentDetail[] }>(
      applicationDeploymentsPath(organizationId, applicationId),
    )
    const items = Array.isArray(body.items) ? body.items : []
    return { kind: 'ok', value: items.map((item) => mapWireDeployment(item)) }
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }
}

export async function loadProductionVariables(
  client: DetailClient,
  organizationId: string,
  applicationId: string,
): Promise<LoadResult<VariableDetail[]>> {
  try {
    const body = await client.get<{ items?: Array<{ id?: string; key?: string; value?: string; scope?: string }> }>(
      applicationVariablesPath(organizationId, applicationId),
    )
    const items = Array.isArray(body.items) ? body.items : []
    return { kind: 'ok', value: items.map(mapWireVariable) }
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }
}

export async function loadProductionDeployment(
  client: DetailClient,
  deploymentId: string,
): Promise<LoadResult<DeploymentDetail>> {
  let wire: WireDeploymentDetail
  try {
    const body = await client.get<{ deployment?: WireDeploymentDetail }>(`/deployments/${deploymentId}`)
    if (!body.deployment?.id) return { kind: 'not-found' }
    wire = body.deployment
  } catch (err) {
    if (isNotFound(err)) return { kind: 'not-found' }
    return { kind: 'error', message: errorMessage(err) }
  }

  const [applicationName, environmentName, serverName] = await Promise.all([
    wire.applicationId
      ? readName(client, `/applications/${wire.applicationId}`, (body) => (body.application as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
    wire.environmentId
      ? readName(client, `/environments/${wire.environmentId}`, (body) => (body.environment as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
    wire.serverId
      ? readName(client, `/servers/${wire.serverId}`, (body) => (body.server as { name?: string } | undefined)?.name)
      : Promise.resolve(null),
  ])

  return { kind: 'ok', value: mapWireDeployment(wire, { applicationName, environmentName, serverName }) }
}

export async function loadApplicationDetail(options: {
  demo: boolean
  applicationId: string
  demoApplication: ApplicationDetail | null
  client: DetailClient
}): Promise<LoadResult<ApplicationDetail>> {
  if (options.demo) {
    return options.demoApplication
      ? { kind: 'ok', value: options.demoApplication }
      : { kind: 'not-found' }
  }
  return loadProductionApplication(options.client, options.applicationId)
}

export async function loadDeploymentDetail(options: {
  demo: boolean
  deploymentId: string
  demoDeployment: DeploymentDetail | null
  client: DetailClient
}): Promise<LoadResult<DeploymentDetail>> {
  if (options.demo) {
    return options.demoDeployment ? { kind: 'ok', value: options.demoDeployment } : { kind: 'not-found' }
  }
  return loadProductionDeployment(options.client, options.deploymentId)
}
