import { API_BASE, type AuthResult, type ErrorEnvelope, type TokenPair } from './contract'

export class ApiError extends Error {
  code: string
  requestId?: string
  details?: Record<string, unknown>
  status: number

  constructor(
    status: number,
    message: string,
    code: string = 'INTERNAL_ERROR',
    requestId?: string,
    details?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
    this.details = details
  }
}

const ACCESS_TOKEN_KEY = 'deploycore_access_token'
const REFRESH_TOKEN_KEY = 'deploycore_refresh_token'
const ORG_KEY = 'deploycore_active_org_id'

export const tokenStorage = {
  get: (): string | null => {
    if (typeof window === 'undefined') return null
    return localStorage.getItem(ACCESS_TOKEN_KEY)
  },
  set: (token: string) => {
    if (typeof window === 'undefined') return
    localStorage.setItem(ACCESS_TOKEN_KEY, token)
  },
  clear: () => {
    if (typeof window === 'undefined') return
    localStorage.removeItem(ACCESS_TOKEN_KEY)
  },
}

/** Refresh tokens are stored in localStorage to match the current JSON token contract (not HttpOnly cookies). */
export const refreshTokenStorage = {
  get: (): string | null => {
    if (typeof window === 'undefined') return null
    return localStorage.getItem(REFRESH_TOKEN_KEY)
  },
  set: (token: string) => {
    if (typeof window === 'undefined') return
    localStorage.setItem(REFRESH_TOKEN_KEY, token)
  },
  clear: () => {
    if (typeof window === 'undefined') return
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  },
}

export const orgStorage = {
  get: (): string | null => {
    if (typeof window === 'undefined') return null
    return localStorage.getItem(ORG_KEY)
  },
  set: (orgId: string) => {
    if (typeof window === 'undefined') return
    localStorage.setItem(ORG_KEY, orgId)
  },
  clear: () => {
    if (typeof window === 'undefined') return
    localStorage.removeItem(ORG_KEY)
  },
}

export function persistTokenPair(tokens: TokenPair | undefined) {
  if (!tokens?.accessToken) return
  tokenStorage.set(tokens.accessToken)
  if (tokens.refreshToken) {
    refreshTokenStorage.set(tokens.refreshToken)
  }
}

export function clearSessionTokens() {
  tokenStorage.clear()
  refreshTokenStorage.clear()
}

interface RequestOptions extends RequestInit {
  orgId?: string
  /** Internal: set after a successful refresh retry to prevent loops. */
  _retry?: boolean
  /** Skip attaching Authorization / org headers (unused auth bootstrap). */
  skipAuth?: boolean
}

function isAuthBootstrapPath(url: string): boolean {
  return /\/auth\/(login|register|refresh|logout|forgot-password|reset-password)(?:\?|$)/.test(url)
}

let refreshInFlight: Promise<boolean> | null = null

/**
 * Exchange refresh token for a new token pair. Uses raw fetch to avoid recursion
 * through the authenticated request path.
 */
export async function refreshAccessToken(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight

  refreshInFlight = (async () => {
    const refreshToken = refreshTokenStorage.get()
    if (!refreshToken) return false

    try {
      const response = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken }),
      })
      if (!response.ok) {
        clearSessionTokens()
        return false
      }
      const data = (await response.json()) as AuthResult
      if (!data.tokens?.accessToken) {
        clearSessionTokens()
        return false
      }
      persistTokenPair(data.tokens)
      return true
    } catch {
      clearSessionTokens()
      return false
    } finally {
      refreshInFlight = null
    }
  })()

  return refreshInFlight
}

async function request<T>(endpoint: string, options: RequestOptions = {}): Promise<T> {
  const url =
    endpoint.startsWith('http') || endpoint.startsWith('/api/')
      ? endpoint
      : `${API_BASE}${endpoint.startsWith('/') ? '' : '/'}${endpoint}`

  const headers = new Headers(options.headers)

  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  if (!options.skipAuth) {
    const token = tokenStorage.get()
    if (token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${token}`)
    }

    const orgId = options.orgId || orgStorage.get()
    if (orgId && !headers.has('X-DeployCore-Organization-Id')) {
      headers.set('X-DeployCore-Organization-Id', orgId)
    }
  }

  const response = await fetch(url, {
    ...options,
    headers,
  })

  if (response.status === 204) {
    return {} as T
  }

  const contentType = response.headers.get('Content-Type') || ''
  const isJson = contentType.includes('application/json')

  if (!response.ok) {
    // Attempt refresh once for expired/invalid access tokens (not for auth bootstrap or authz).
    if (
      response.status === 401 &&
      !options._retry &&
      !options.skipAuth &&
      !isAuthBootstrapPath(url) &&
      refreshTokenStorage.get()
    ) {
      // Drain body before retry.
      await response.text().catch(() => '')
      const refreshed = await refreshAccessToken()
      if (refreshed) {
        return request<T>(endpoint, { ...options, _retry: true })
      }
    }

    if (isJson) {
      try {
        const errorData = (await response.json()) as ErrorEnvelope
        if (errorData?.error) {
          throw new ApiError(
            response.status,
            safeOperatorMessage(errorData.error.message, response.status),
            errorData.error.code || 'UNKNOWN_ERROR',
            errorData.error.requestId,
            errorData.error.details,
          )
        }
      } catch (err) {
        if (err instanceof ApiError) throw err
      }
    }
    await response.text().catch(() => '')
    throw new ApiError(
      response.status,
      safeOperatorMessage(undefined, response.status),
      statusToCode(response.status),
    )
  }

  if (!isJson) {
    return (await response.text()) as unknown as T
  }

  return response.json() as Promise<T>
}

export const apiClient = {
  get: <T>(endpoint: string, options?: RequestOptions) =>
    request<T>(endpoint, { ...options, method: 'GET' }),

  post: <T>(endpoint: string, body?: unknown, options?: RequestOptions) =>
    request<T>(endpoint, {
      ...options,
      method: 'POST',
      body: body instanceof FormData ? body : JSON.stringify(body ?? {}),
    }),

  put: <T>(endpoint: string, body?: unknown, options?: RequestOptions) =>
    request<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: body instanceof FormData ? body : JSON.stringify(body ?? {}),
    }),

  patch: <T>(endpoint: string, body?: unknown, options?: RequestOptions) =>
    request<T>(endpoint, {
      ...options,
      method: 'PATCH',
      body: body instanceof FormData ? body : JSON.stringify(body ?? {}),
    }),

  delete: <T>(endpoint: string, options?: RequestOptions) =>
    request<T>(endpoint, { ...options, method: 'DELETE' }),
}

function safeOperatorMessage(candidate: string | undefined, status: number): string {
  const trimmed = (candidate || '').trim()
  if (
    trimmed &&
    trimmed.length <= 280 &&
    !trimmed.startsWith('<!') &&
    !/<html[\s>]/i.test(trimmed) &&
    !trimmed.includes('\n<html')
  ) {
    return trimmed
  }
  if (status === 401 || status === 403) {
    return 'The API request was not authorized. Please sign in again and retry.'
  }
  if (status === 404) {
    return 'The requested resource was not found.'
  }
  if (status === 429) {
    return 'Too many requests. Please wait and retry.'
  }
  if (status >= 500) {
    return 'The API request failed. Please retry.'
  }
  return 'The API request failed. Please retry.'
}

function statusToCode(status: number): string {
  if (status === 401) return 'UNAUTHORIZED'
  if (status === 403) return 'FORBIDDEN'
  if (status === 404) return 'RESOURCE_NOT_FOUND'
  if (status === 429) return 'RATE_LIMITED'
  if (status >= 500) return 'SERVICE_UNAVAILABLE'
  return 'INTERNAL_ERROR'
}
