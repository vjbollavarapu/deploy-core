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
  get<T>(path: string): Promise<T>
}

export type VolumeInput = {
  name: string
  mountPath: string
  readOnly: boolean
}

/** Non-secret progress for a volume that has a control-plane row. */
export type SavedVolume = {
  name: string
  volumeId: string
  mountPath: string
  readOnly: boolean
  attached: boolean
}

export class CreateFlowError extends Error {
  readonly phase: 'application' | 'configuration' | 'storage' | 'deployment'
  readonly applicationId?: string
  /** Variable keys confirmed by a successful POST /variables. Values are not stored. */
  readonly savedKeys: string[]
  /** Volume rows already created. Names and ids only; no secret values. */
  readonly savedVolumes: SavedVolume[]

  constructor(
    phase: 'application' | 'configuration' | 'storage' | 'deployment',
    message: string,
    applicationId?: string,
    savedKeys: string[] = [],
    savedVolumes: SavedVolume[] = [],
  ) {
    super(message)
    this.name = 'CreateFlowError'
    this.phase = phase
    this.applicationId = applicationId
    this.savedKeys = savedKeys
    this.savedVolumes = savedVolumes
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

type VolumeRecord = {
  id?: string
  state?: string
  name?: string
  serverId?: string
  dockerName?: string | null
  attachedResourceId?: string | null
  mountPath?: string
}

const volumePollIntervalMs = 400
const volumePollTimeoutMs = 90_000

function sleep(ms: number): Promise<void> {
  if (ms <= 0) return Promise.resolve()
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function sameVolumeName(left: string, right: string): boolean {
  return left.trim().toLowerCase() === right.trim().toLowerCase()
}

async function findVolumeByName(
  client: FlowClient,
  organizationId: string,
  serverId: string,
  name: string,
): Promise<VolumeRecord | undefined> {
  const page = await client.get<{ items?: VolumeRecord[] }>(
    `/volumes?organizationId=${encodeURIComponent(organizationId)}&serverId=${encodeURIComponent(serverId)}&limit=100`,
  )
  return (page.items ?? []).find((item) => item.name && sameVolumeName(item.name, name) && item.state !== 'DELETED')
}

function canRetryFailedVolume(volume: VolumeRecord): boolean {
  const dockerName = (volume.dockerName ?? '').trim()
  const attachedID = (volume.attachedResourceId ?? '').trim()
  return volume.state === 'FAILED' && dockerName.length === 0 && attachedID.length === 0
}

async function waitForVolumeReady(
  client: FlowClient,
  volumeId: string,
  applicationId: string,
): Promise<VolumeRecord> {
  const started = Date.now()
  let latest = 'PENDING'
  for (;;) {
    const res = await client.get<{ volume?: VolumeRecord }>(`/volumes/${volumeId}`)
    const volume = res.volume
    latest = volume?.state ?? latest
    if (latest === 'FAILED') {
      return volume ?? { id: volumeId, state: 'FAILED' }
    }
    if (latest === 'READY' || (latest === 'ATTACHED' && volume?.attachedResourceId === applicationId)) {
      return volume ?? { id: volumeId, state: latest }
    }
    if (Date.now() - started > volumePollTimeoutMs) {
      throw new Error(`volume is still ${latest}`)
    }
    await sleep(volumePollIntervalMs)
  }
}

async function ensureApplicationVolumes(
  client: FlowClient,
  input: {
    organizationId: string
    applicationId: string
    serverId: string
    volumes: VolumeInput[]
    saved: SavedVolume[]
    savedKeys: string[]
    secretValues: string[]
  },
): Promise<SavedVolume[]> {
  const wanted = input.volumes
    .map((volume) => ({
      name: volume.name.trim(),
      mountPath: volume.mountPath.trim(),
      readOnly: volume.readOnly,
    }))
    .filter((volume) => volume.name.length > 0)

  if (wanted.length > 0 && !input.serverId) {
    throw new CreateFlowError(
      'storage',
      'Select a target server before creating storage. Deployment was not queued.',
      input.applicationId,
      input.savedKeys,
      input.saved,
    )
  }

  const seen = new Set<string>()
  for (const volume of wanted) {
    const key = volume.name.toLowerCase()
    if (seen.has(key)) {
      throw new CreateFlowError(
        'storage',
        `Volume ${volume.name} is listed more than once. Deployment was not queued.`,
        input.applicationId,
        input.savedKeys,
        input.saved,
      )
    }
    seen.add(key)
  }

  for (const saved of input.saved) {
    if (!wanted.some((volume) => sameVolumeName(volume.name, saved.name))) {
      throw new CreateFlowError(
        'storage',
        `Saved volume ${saved.name} is no longer in this wizard and was not changed. Deployment was not queued.`,
        input.applicationId,
        input.savedKeys,
        input.saved,
      )
    }
  }

  const progress = input.saved.map((volume) => ({ ...volume }))

  for (const volume of wanted) {
    let current = progress.find((item) => sameVolumeName(item.name, volume.name))
    if (!current) {
      let volumeId = ''
      try {
        const created = await client.post<{ volume?: VolumeRecord }>('/volumes', {
          organizationId: input.organizationId,
          serverId: input.serverId,
          name: volume.name,
          mountPath: volume.mountPath,
          labels: { readOnly: volume.readOnly },
        })
        volumeId = created.volume?.id ?? ''
      } catch (err) {
        const detail = redactVariableValues(errorText(err), input.secretValues)
        if (!/already exists/i.test(detail)) {
          throw new CreateFlowError(
            'storage',
            `Volume ${volume.name} could not be created (${detail}). Deployment was not queued. Press Deploy again to retry storage.`,
            input.applicationId,
            input.savedKeys,
            progress,
          )
        }
        const existing = await findVolumeByName(client, input.organizationId, input.serverId, volume.name)
        volumeId = existing?.id ?? ''
      }
      if (!volumeId) {
        throw new CreateFlowError(
          'storage',
          `Volume ${volume.name} was created without an id. Deployment was not queued.`,
          input.applicationId,
          input.savedKeys,
          progress,
        )
      }
      current = {
        name: volume.name,
        volumeId,
        mountPath: volume.mountPath,
        readOnly: volume.readOnly,
        attached: false,
      }
      progress.push(current)
    }

    if (!current.attached) {
      let ready: VolumeRecord
      try {
        ready = await waitForVolumeReady(client, current.volumeId, input.applicationId)
      } catch (err) {
        const detail = redactVariableValues(errorText(err), input.secretValues)
        throw new CreateFlowError(
          'storage',
          `Volume ${volume.name} is not ready (${detail}). Deployment was not queued. Press Deploy again to keep waiting on this volume.`,
          input.applicationId,
          input.savedKeys,
          progress,
        )
      }
      if (ready.state === 'FAILED' && canRetryFailedVolume(ready)) {
        try {
          await client.post(`/volumes/${current.volumeId}/retry`, {})
        } catch (err) {
          const detail = redactVariableValues(errorText(err), input.secretValues)
          throw new CreateFlowError(
            'storage',
            `Volume ${volume.name} failed to create and could not be retried (${detail}). Deployment was not queued.`,
            input.applicationId,
            input.savedKeys,
            progress,
          )
        }
        try {
          ready = await waitForVolumeReady(client, current.volumeId, input.applicationId)
        } catch (err) {
          const detail = redactVariableValues(errorText(err), input.secretValues)
          throw new CreateFlowError(
            'storage',
            `Volume ${volume.name} is not ready (${detail}). Deployment was not queued. Press Deploy again to keep waiting on this volume.`,
            input.applicationId,
            input.savedKeys,
            progress,
          )
        }
      }
      if (ready.state === 'FAILED') {
        throw new CreateFlowError(
          'storage',
          `Volume ${volume.name} failed to create. Deployment was not queued.`,
          input.applicationId,
          input.savedKeys,
          progress,
        )
      }
      if (ready.state === 'ATTACHED' && ready.attachedResourceId === input.applicationId) {
        current.attached = true
        current.mountPath = volume.mountPath
        current.readOnly = volume.readOnly
        continue
      }
      try {
        await client.post(`/volumes/${current.volumeId}/attach`, {
          resourceType: 'application',
          resourceId: input.applicationId,
          mountPath: volume.mountPath,
          readOnly: volume.readOnly,
        })
        current.attached = true
        current.mountPath = volume.mountPath
        current.readOnly = volume.readOnly
      } catch (err) {
        const detail = redactVariableValues(errorText(err), input.secretValues)
        throw new CreateFlowError(
          'storage',
          `Volume ${volume.name} was created, but it could not be attached (${detail}). Deployment was not queued. Press Deploy again to retry attachment.`,
          input.applicationId,
          input.savedKeys,
          progress,
        )
      }
    }
  }

  return progress
}

export async function createApplicationWithVariables(
  client: FlowClient,
  input: {
    applicationBody: unknown
    organizationId: string
    serverId?: string
    envVars: EnvVariableInput[]
    volumes?: VolumeInput[]
    resume?: { applicationId: string; savedKeys: string[]; savedVolumes?: SavedVolume[] }
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
  const resumedVolumes = (input.resume?.savedVolumes ?? []).map((volume) => ({ ...volume }))

  const seen = new Set<string>()
  for (const body of bodies) {
    if (seen.has(body.key)) {
      throw new CreateFlowError(
        'configuration',
        `Application already exists. Environment variable ${body.key} is listed more than once. Deployment was not queued. Press Deploy again after keeping one row for that key.`,
        applicationId,
        [...saved],
        resumedVolumes,
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
        resumedVolumes,
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
      throw new CreateFlowError('configuration', message, applicationId, savedKeys, resumedVolumes)
    }
  }

  const savedVolumes = await ensureApplicationVolumes(client, {
    organizationId: input.organizationId,
    applicationId,
    serverId: input.serverId ?? '',
    volumes: input.volumes ?? [],
    saved: resumedVolumes,
    savedKeys: [...saved],
    secretValues: values,
  })

  try {
    await client.post(`/applications/${applicationId}/deployments`, { trigger: 'manual' })
  } catch (err) {
    const detail = redactVariableValues(errorText(err), values)
    const savedSummary =
      savedVolumes.length > 0
        ? 'Application, environment variables, and storage were saved'
        : 'Application and environment variables were saved'
    throw new CreateFlowError(
      'deployment',
      `${savedSummary}, but the deployment could not be queued (${detail}). Press Deploy again to retry the deployment.`,
      applicationId,
      bodies.map((body) => body.key),
      savedVolumes,
    )
  }

  return { applicationId }
}

export const CREATE_APPLICATION_RESUME_KEY = 'deploycore.create-application.resume'

export type CreateApplicationResume = {
  applicationId: string
  savedKeys: string[]
  savedVolumes: SavedVolume[]
  volumes: Array<{ name: string; mountPath: string; writable: boolean }>
}

export function readCreateApplicationResume(): CreateApplicationResume | null {
  if (typeof window === 'undefined') return null
  const raw = window.sessionStorage.getItem(CREATE_APPLICATION_RESUME_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as CreateApplicationResume
    if (!parsed || typeof parsed.applicationId !== 'string' || !parsed.applicationId) return null
    return {
      applicationId: parsed.applicationId,
      savedKeys: Array.isArray(parsed.savedKeys) ? parsed.savedKeys.filter((key) => typeof key === 'string') : [],
      savedVolumes: Array.isArray(parsed.savedVolumes) ? parsed.savedVolumes : [],
      volumes: Array.isArray(parsed.volumes) ? parsed.volumes : [],
    }
  } catch {
    return null
  }
}

export function writeCreateApplicationResume(resume: CreateApplicationResume): void {
  if (typeof window === 'undefined') return
  window.sessionStorage.setItem(CREATE_APPLICATION_RESUME_KEY, JSON.stringify(resume))
}

export function clearCreateApplicationResume(): void {
  if (typeof window === 'undefined') return
  window.sessionStorage.removeItem(CREATE_APPLICATION_RESUME_KEY)
}
