import { safeInternalPath } from '../auth/internal-path'
import type {
  CompleteGitHubInstallationRequest,
  CompleteGitHubInstallationResponse,
  WireGitConnection,
} from './git-connection'

export const GITHUB_APP_CALLBACK_PATH = '/integrations/git/github/callback'
export const GIT_PROVIDERS_PATH = '/integrations/git'

const CONNECTION_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export type GitHubCallbackOutcome =
  | {
      kind: 'SUCCESS'
      connection: WireGitConnection
      repositoryCount: number
    }
  | { kind: 'NOT_FINISHED' }
  | { kind: 'INVALID_CALLBACK' }
  | { kind: 'EXPIRED_OR_INVALID' }
  | { kind: 'ALREADY_USED' }
  | { kind: 'ALREADY_CONNECTED_ELSEWHERE' }
  | { kind: 'SUSPENDED'; connectionId: string | null }
  | { kind: 'SYNC_FAILED'; connectionId: string }
  | { kind: 'PERMISSION_DENIED' }
  | { kind: 'APP_NOT_CONFIGURED' }
  | { kind: 'GITHUB_UNAVAILABLE' }
  | { kind: 'UNKNOWN_ERROR' }

export interface GitHubCallbackView {
  title: string
  message: string
  tone: 'success' | 'error'
  showTryAgain: boolean
  showReturn: boolean
  retryConnectionId: string | null
}

type CallbackPlan =
  | { action: 'complete'; body: CompleteGitHubInstallationRequest }
  | { action: 'local'; outcome: GitHubCallbackOutcome }

/**
 * Decide whether a GitHub return may call the complete endpoint.
 * `setup_action=request` and a missing installation id never call the API.
 */
export function planGitHubCallback(search: string): CallbackPlan {
  const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
  const setupAction = params.get('setup_action') ?? ''
  const installationRaw = params.get('installation_id')
  const state = (params.get('state') ?? '').trim()

  if (setupAction === 'request' || installationRaw == null || installationRaw.trim() === '') {
    return { action: 'local', outcome: { kind: 'NOT_FINISHED' } }
  }
  if (!state) {
    return { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } }
  }
  if (!/^[1-9][0-9]*$/.test(installationRaw) || !Number.isSafeInteger(Number(installationRaw))) {
    return { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } }
  }
  if (setupAction !== 'install' && setupAction !== 'update') {
    return { action: 'local', outcome: { kind: 'INVALID_CALLBACK' } }
  }
  return {
    action: 'complete',
    body: {
      installationId: Number(installationRaw),
      setupAction,
      state,
    },
  }
}

export async function settleGitHubCallback(
  search: string,
  complete: (body: CompleteGitHubInstallationRequest) => Promise<CompleteGitHubInstallationResponse>,
): Promise<GitHubCallbackOutcome> {
  const plan = planGitHubCallback(search)
  if (plan.action === 'local') return plan.outcome
  try {
    return mapGitHubCompleteSuccess(await complete(plan.body))
  } catch (error) {
    return mapGitHubCallbackError(error)
  }
}

export function mapGitHubCompleteSuccess(
  result: CompleteGitHubInstallationResponse | null | undefined,
): GitHubCallbackOutcome {
  if (!result?.connection || typeof result.connection.id !== 'string' || result.connection.id === '') {
    return { kind: 'UNKNOWN_ERROR' }
  }
  if (typeof result.repositoryCount !== 'number' || !Number.isFinite(result.repositoryCount)) {
    return { kind: 'UNKNOWN_ERROR' }
  }
  return {
    kind: 'SUCCESS',
    connection: result.connection,
    repositoryCount: result.repositoryCount,
  }
}

export function mapGitHubCallbackError(error: unknown): GitHubCallbackOutcome {
  const parsed = readError(error)
  if (!parsed) return { kind: 'UNKNOWN_ERROR' }
  const message = (parsed.message ?? '').toLowerCase()
  const connectionId = connectionIdFrom(parsed.details)
  const detailStatus = typeof parsed.details?.status === 'string' ? parsed.details.status : ''

  if (parsed.status === 403 || parsed.code === 'FORBIDDEN') {
    return { kind: 'PERMISSION_DENIED' }
  }
  if (message.includes('github app is not configured')) {
    return { kind: 'APP_NOT_CONFIGURED' }
  }
  if (
    (parsed.status === 502 || parsed.status === 503) &&
    connectionId &&
    detailStatus === 'error'
  ) {
    return { kind: 'SYNC_FAILED', connectionId }
  }
  if (parsed.status === 409 && (detailStatus === 'disabled' || message.includes('suspended'))) {
    return { kind: 'SUSPENDED', connectionId }
  }
  if (parsed.status === 409 && (message.includes('already been used') || message.includes('already used'))) {
    return { kind: 'ALREADY_USED' }
  }
  if (parsed.status === 409 && message.includes('another organization')) {
    return { kind: 'ALREADY_CONNECTED_ELSEWHERE' }
  }
  if (
    parsed.status === 400 &&
    (message.includes('installation state is invalid') ||
      message.includes('installation state has expired') ||
      message.includes('state is invalid') ||
      message.includes('state has expired'))
  ) {
    return { kind: 'EXPIRED_OR_INVALID' }
  }
  if (parsed.status === 400) {
    return { kind: 'INVALID_CALLBACK' }
  }
  if (parsed.status === 404 || parsed.status === 429 || parsed.status === 502 || parsed.code === 'RATE_LIMITED') {
    return { kind: 'GITHUB_UNAVAILABLE' }
  }
  if (parsed.status === 503) {
    return { kind: 'GITHUB_UNAVAILABLE' }
  }
  return { kind: 'UNKNOWN_ERROR' }
}

export function githubCallbackView(outcome: GitHubCallbackOutcome): GitHubCallbackView {
  switch (outcome.kind) {
    case 'SUCCESS':
      return {
        title: 'Connected successfully',
        message: successMessage(outcome.connection, outcome.repositoryCount),
        tone: 'success',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'NOT_FINISHED':
      return {
        title: 'GitHub installation was not finished',
        message: 'GitHub did not finish installing the app. Start again from Git Providers when you are ready.',
        tone: 'error',
        showTryAgain: true,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'EXPIRED_OR_INVALID':
      return {
        title: 'Installation link expired',
        message: 'This installation link is no longer valid. Start Connect GitHub again from Git Providers.',
        tone: 'error',
        showTryAgain: true,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'ALREADY_USED':
      return {
        title: 'Installation link was already used',
        message: 'This installation link was already used. Start again from Git Providers if the account is not connected.',
        tone: 'error',
        showTryAgain: true,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'ALREADY_CONNECTED_ELSEWHERE':
      return {
        title: 'Installation belongs elsewhere',
        message: 'This GitHub installation is already connected to another DeployCore organization.',
        tone: 'error',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'SUSPENDED':
      return {
        title: 'GitHub installation is suspended',
        message: 'GitHub reports this installation as suspended. Unsuspend it on GitHub, then sync repositories from Git Providers.',
        tone: 'error',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'SYNC_FAILED':
      return {
        title: 'GitHub connected but repositories could not be synchronized',
        message: 'The GitHub account is connected. Repository sync did not finish. Retry sync, or return to Git Providers.',
        tone: 'error',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: outcome.connectionId,
      }
    case 'PERMISSION_DENIED':
      return {
        title: 'Permission denied',
        message: 'You do not have permission to connect GitHub for this organization.',
        tone: 'error',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'APP_NOT_CONFIGURED':
      return {
        title: 'GitHub App is not configured',
        message: 'An administrator must configure the GitHub App for this DeployCore installation before accounts can be connected.',
        tone: 'error',
        showTryAgain: false,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'GITHUB_UNAVAILABLE':
      return {
        title: 'GitHub could not be reached',
        message: 'GitHub did not complete the request. Wait a moment and try again from Git Providers.',
        tone: 'error',
        showTryAgain: true,
        showReturn: true,
        retryConnectionId: null,
      }
    case 'INVALID_CALLBACK':
    case 'UNKNOWN_ERROR':
      return {
        title: 'Something went wrong',
        message: 'This GitHub installation could not be completed. Return to Git Providers and start again.',
        tone: 'error',
        showTryAgain: true,
        showReturn: true,
        retryConnectionId: null,
      }
  }
}

export function matchingOrganization<T extends { id?: string }>(
  organizationId: string | undefined,
  organizations: readonly T[],
): T | null {
  if (!organizationId) return null
  return organizations.find((org) => org.id === organizationId) ?? null
}

/** Drop opaque installation state from analytics URLs, including a login `next` path. */
export function sanitizeAnalyticsUrl(raw: string): string {
  const absolute = /^[a-z][a-z0-9+.-]*:/i.test(raw)
  let url: URL
  try {
    url = new URL(raw, 'http://localhost')
  } catch {
    return raw.includes('state=') ? raw.split('?')[0] ?? raw : raw
  }
  if (url.pathname === GITHUB_APP_CALLBACK_PATH) {
    url.search = ''
    url.hash = ''
  } else {
    url.searchParams.delete('state')
    const next = url.searchParams.get('next')
    if (next) {
      const cleaned = stripStateFromInternalPath(next)
      if (cleaned) url.searchParams.set('next', cleaned)
      else url.searchParams.delete('next')
    }
  }
  if (!absolute) return `${url.pathname}${url.search}${url.hash}`
  return url.toString()
}

export function sanitizeAnalyticsEvent<T extends { url: string }>(event: T): T {
  return { ...event, url: sanitizeAnalyticsUrl(event.url) }
}

function successMessage(connection: WireGitConnection, repositoryCount: number): string {
  const repos =
    repositoryCount === 1 ? '1 repository is available.' : `${repositoryCount} repositories are available.`
  const login = connection.accountLogin?.trim()
  if (login) return `${login} is connected. ${repos}`
  return `GitHub is connected. ${repos}`
}

function stripStateFromInternalPath(path: string): string | null {
  const safe = safeInternalPath(path)
  if (!safe) return null
  const splitAt = safe.indexOf('?')
  if (splitAt === -1) return safe
  const params = new URLSearchParams(safe.slice(splitAt + 1))
  params.delete('state')
  const rest = params.toString()
  return rest ? `${safe.slice(0, splitAt)}?${rest}` : safe.slice(0, splitAt)
}

function connectionIdFrom(details: Record<string, unknown> | undefined): string | null {
  const value = details?.connectionId
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return CONNECTION_ID.test(trimmed) ? trimmed : null
}

function readError(error: unknown): {
  status: number
  code?: string
  message?: string
  details?: Record<string, unknown>
} | null {
  if (!error || typeof error !== 'object') return null
  const value = error as {
    status?: unknown
    code?: unknown
    message?: unknown
    details?: unknown
  }
  if (typeof value.status !== 'number') return null
  const details =
    value.details && typeof value.details === 'object' && !Array.isArray(value.details)
      ? (value.details as Record<string, unknown>)
      : undefined
  return {
    status: value.status,
    code: typeof value.code === 'string' ? value.code : undefined,
    message: typeof value.message === 'string' ? value.message : undefined,
    details,
  }
}
