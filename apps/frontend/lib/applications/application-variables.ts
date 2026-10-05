/** Application-scoped environment variable mutations. Values stay out of messages. */

const VARIABLE_KEY = /^[A-Za-z_][A-Za-z0-9_]*$/
const MAX_KEY_LENGTH = 256
const MAX_VALUE_LENGTH = 8192

export interface VariableMutationClient {
  post<T>(path: string, body?: unknown): Promise<T>
  patch<T>(path: string, body?: unknown): Promise<T>
  delete<T>(path: string): Promise<T>
}

export interface ApplicationVariableWriteInput {
  organizationId: string
  applicationId: string
  key: string
  value: string
}

export interface ApplicationVariableFieldErrors {
  key?: string
  value?: string
}

export function applicationVariableFieldErrors(key: string, value: string): ApplicationVariableFieldErrors {
  const errors: ApplicationVariableFieldErrors = {}
  const trimmed = key.trim()
  if (!trimmed) {
    errors.key = 'Key is required.'
  } else if (trimmed.length > MAX_KEY_LENGTH) {
    errors.key = 'Key must be at most 256 characters.'
  } else if (!VARIABLE_KEY.test(trimmed)) {
    errors.key = 'Key must match [A-Za-z_][A-Za-z0-9_]*.'
  }
  if (value.length > MAX_VALUE_LENGTH) {
    errors.value = 'Value must be at most 8192 characters.'
  }
  return errors
}

export function applicationVariableCreateBody(input: ApplicationVariableWriteInput) {
  return {
    organizationId: input.organizationId,
    scope: 'APPLICATION' as const,
    applicationId: input.applicationId,
    key: input.key.trim(),
    value: input.value,
  }
}

export function applicationVariableUpdateBody(input: { key: string; value: string }) {
  return {
    key: input.key.trim(),
    value: input.value,
  }
}

export function applicationVariablePath(variableId: string): string {
  return `/variables/${encodeURIComponent(variableId)}`
}

export async function createApplicationVariable(
  client: VariableMutationClient,
  input: ApplicationVariableWriteInput,
): Promise<void> {
  await client.post('/variables', applicationVariableCreateBody(input))
}

export async function updateApplicationVariable(
  client: VariableMutationClient,
  variableId: string,
  input: { key: string; value: string },
): Promise<void> {
  await client.patch(applicationVariablePath(variableId), applicationVariableUpdateBody(input))
}

export async function deleteApplicationVariable(
  client: VariableMutationClient,
  variableId: string,
): Promise<void> {
  await client.delete(applicationVariablePath(variableId))
}

export function variableErrorMessage(
  err: unknown,
  values: readonly string[],
  fallback = 'The variable could not be saved.',
): string {
  const raw = err instanceof Error && err.message.trim() ? err.message.trim() : fallback
  for (const value of values) {
    if (value.length >= 3 && raw.includes(value)) return fallback
  }
  return raw
}
