/** Application-scoped secret metadata and the explicit first deployment. */

export interface BootstrapClient {
  get<T>(path: string): Promise<T>
  post<T>(path: string, body?: unknown): Promise<T>
}

export interface SecretMetadata {
  id: string
  name: string
  scope: string
  version: number | null
  applicationId: string | null
  updatedAt: string | null
}

export interface ApplicationSecretCreateInput {
  organizationId: string
  applicationId: string
  name: string
  value: string
}

export function applicationSecretsListPath(organizationId: string, applicationId: string): string {
  const params = new URLSearchParams({
    organizationId,
    scope: 'APPLICATION',
    applicationId,
  })
  return `/secrets?${params.toString()}`
}

export function applicationSecretCreateBody(input: ApplicationSecretCreateInput) {
  return {
    organizationId: input.organizationId,
    scope: 'APPLICATION' as const,
    applicationId: input.applicationId,
    name: input.name.trim(),
    value: input.value,
  }
}

export function mapSecretMetadata(wire: {
  id?: string
  name?: string
  scope?: string
  version?: number
  applicationId?: string | null
  updatedAt?: string
  value?: string
}): SecretMetadata {
  return {
    id: wire.id ?? '',
    name: wire.name ?? '',
    scope: wire.scope ?? '',
    version: typeof wire.version === 'number' ? wire.version : null,
    applicationId: wire.applicationId ?? null,
    updatedAt: wire.updatedAt ?? null,
  }
}

export async function loadApplicationSecrets(
  client: BootstrapClient,
  organizationId: string,
  applicationId: string,
): Promise<SecretMetadata[]> {
  const body = await client.get<{ items?: Array<Parameters<typeof mapSecretMetadata>[0]> }>(
    applicationSecretsListPath(organizationId, applicationId),
  )
  const items = Array.isArray(body.items) ? body.items : []
  return items.map((item) => mapSecretMetadata(item))
}

export async function createApplicationSecret(
  client: BootstrapClient,
  input: ApplicationSecretCreateInput,
): Promise<SecretMetadata> {
  const body = applicationSecretCreateBody(input)
  const response = await client.post<{ secret?: Parameters<typeof mapSecretMetadata>[0] }>('/secrets', body)
  return mapSecretMetadata(response.secret ?? {})
}

export function manualDeploymentPath(applicationId: string): string {
  return `/applications/${encodeURIComponent(applicationId)}/deployments`
}

export function manualDeploymentBody() {
  return { trigger: 'manual' as const }
}

export function deployActionLabel(status: string | null | undefined): 'Deploy' | 'Redeploy' {
  return (status ?? '').toLowerCase() === 'draft' ? 'Deploy' : 'Redeploy'
}

export interface DeploymentInflight {
  current: boolean
}

export async function queueManualDeployment(
  client: BootstrapClient,
  applicationId: string,
  inflight: DeploymentInflight,
): Promise<{ kind: 'ignored' } | { kind: 'ok'; deploymentId: string | null }> {
  if (inflight.current) return { kind: 'ignored' }
  inflight.current = true
  try {
    const body = await client.post<{ deployment?: { id?: string } }>(
      manualDeploymentPath(applicationId),
      manualDeploymentBody(),
    )
    return { kind: 'ok', deploymentId: body.deployment?.id ?? null }
  } finally {
    inflight.current = false
  }
}
