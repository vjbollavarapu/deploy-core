/**
 * Production create-application sequence.
 * Persist application-scoped environment variables before the first deployment
 * so revision creation can snapshot them.
 */

export type EnvVariableInput = {
  key: string
  value: string
}

export type ApplicationVariableBody = {
  organizationId: string
  scope: 'APPLICATION'
  applicationId: string
  key: string
  value: string
}

export type CreationMode = 'api' | 'demo' | 'blocked'

export type FlowClient = {
  post<T>(path: string, body?: unknown): Promise<T>
}

export class CreateFlowError extends Error {
  readonly phase: 'application' | 'configuration' | 'deployment'
  readonly applicationId?: string
  /** Variable keys confirmed by a successful POST /variables. Values are not stored. */
  readonly savedKeys: string[]

  constructor(
    phase: 'application' | 'configuration' | 'deployment',
    message: string,
    applicationId?: string,
    savedKeys: string[] = [],
  ) {
    super(message)
    this.name = 'CreateFlowError'
    this.phase = phase
    this.applicationId = applicationId
    this.savedKeys = savedKeys
  }
}

export function isPersistedVariableKey(savedKeys: readonly string[], key: string): boolean {
  const trimmed = key.trim()
  return trimmed.length > 0 && savedKeys.includes(trimmed)
}

export function creationMode(hasControlPlaneIds: boolean, demoMode: boolean): CreationMode {
  if (hasControlPlaneIds) return 'api'
  if (demoMode) return 'demo'
  return 'blocked'
}

export function applicationVariableBodies(
  organizationId: string,
  applicationId: string,
  envVars: EnvVariableInput[],
): ApplicationVariableBody[] {
  return envVars
    .map((entry) => ({ key: entry.key.trim(), value: entry.value }))
    .filter((entry) => entry.key.length > 0)
    .map((entry) => ({
      organizationId,
      scope: 'APPLICATION',
      applicationId,
      key: entry.key,
      value: entry.value,
    }))
}

export function redactVariableValues(message: string, values: string[]): string {
  const secrets = [...new Set(values.filter((value) => value.length > 0))].sort(
    (a, b) => b.length - a.length,
  )
  let out = message
  for (const secret of secrets) {
    out = out.split(secret).join('[REDACTED]')
  }
  return out
}

function errorText(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return 'request failed'
}

export async function createApplicationWithVariables(
  client: FlowClient,
  input: {
    applicationBody: unknown
    organizationId: string
    envVars: EnvVariableInput[]
    resume?: { applicationId: string; savedKeys: string[] }
  },
): Promise<{ applicationId: string }> {
  let applicationId = input.resume?.applicationId
  if (!applicationId) {
    const created = await client.post<{ application?: { id?: string } }>('/applications', input.applicationBody)
    applicationId = created.application?.id
    if (!applicationId) {
      throw new CreateFlowError(
        'application',
        'Application was created without an id. Deployment was not queued.',
      )
    }
  }

  const bodies = applicationVariableBodies(input.organizationId, applicationId, input.envVars)
  const values = bodies.map((body) => body.value)
  const saved = new Set(input.resume?.savedKeys ?? [])

  const seen = new Set<string>()
  for (const body of bodies) {
    if (seen.has(body.key)) {
      throw new CreateFlowError(
        'configuration',
        `Application already exists. Environment variable ${body.key} is listed more than once. Deployment was not queued. Press Deploy again after keeping one row for that key.`,
        applicationId,
        [...saved],
      )
    }
    seen.add(body.key)
  }

  for (const key of saved) {
    if (!seen.has(key)) {
      throw new CreateFlowError(
        'configuration',
        `Application already exists. Saved environment variable ${key} is no longer in this wizard and was not changed. Deployment was not queued.`,
        applicationId,
        [...saved],
      )
    }
  }

  for (const body of bodies) {
    if (saved.has(body.key)) continue
    try {
      await client.post('/variables', body)
      saved.add(body.key)
    } catch (err) {
      const detail = redactVariableValues(errorText(err), values)
      const savedKeys = [...saved]
      const message =
        savedKeys.length > 0
          ? `Application already exists. Some configuration was saved, but environment variable ${body.key} could not be saved (${detail}). Deployment was not queued. Press Deploy again to retry the remaining configuration.`
          : `Application was created, but environment variable ${body.key} could not be saved (${detail}). Deployment was not queued. Press Deploy again to retry the remaining configuration.`
      throw new CreateFlowError('configuration', message, applicationId, savedKeys)
    }
  }

  try {
    await client.post(`/applications/${applicationId}/deployments`, { trigger: 'manual' })
  } catch (err) {
    const detail = redactVariableValues(errorText(err), values)
    throw new CreateFlowError(
      'deployment',
      `Application and environment variables were saved, but the deployment could not be queued (${detail}). Retry deployment from the application.`,
      applicationId,
      bodies.map((body) => body.key),
    )
  }

  return { applicationId }
}
