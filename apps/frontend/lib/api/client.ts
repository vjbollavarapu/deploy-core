import { API_BASE, type ErrorEnvelope } from './contract'

export class ApiError extends Error {
  code: string
  requestId?: string
  details?: Record<string, unknown>
  status: number

  constructor(status: number, message: string, code: string = 'INTERNAL_ERROR', requestId?: string, details?: Record<string, unknown>) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
    this.details = details
  }
}

const TOKEN_KEY = 'deploycore_access_token'
const ORG_KEY = 'deploycore_active_org_id'

export const tokenStorage = {
  get: (): string | null => {
    if (typeof window === 'undefined') return null
    return localStorage.getItem(TOKEN_KEY)
  },
  set: (token: string) => {
    if (typeof window === 'undefined') return
    localStorage.setItem(TOKEN_KEY, token)
  },
  clear: () => {
    if (typeof window === 'undefined') return
    localStorage.removeItem(TOKEN_KEY)
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

interface RequestOptions extends RequestInit {
  orgId?: string
}

async function request<T>(endpoint: string, options: RequestOptions = {}): Promise<T> {
  const url = endpoint.startsWith('http') || endpoint.startsWith('/api/')
    ? endpoint
    : `${API_BASE}${endpoint.startsWith('/') ? '' : '/'}${endpoint}`

  const headers = new Headers(options.headers)

  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  const token = tokenStorage.get()
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const orgId = options.orgId || orgStorage.get()
  if (orgId && !headers.has('X-DeployCore-Organization-Id')) {
    headers.set('X-DeployCore-Organization-Id', orgId)
  }

  const response = await fetch(url, {
    ...options,
    headers,
  })

  // Handle 204 No Content
  if (response.status === 204) {
    return {} as T
  }

  const contentType = response.headers.get('Content-Type') || ''
  const isJson = contentType.includes('application/json')

  if (!response.ok) {
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
    // Never surface raw HTML/proxy bodies to the UI (I12).
    await response.text().catch(() => '')
    throw new ApiError(
      response.status,
      safeOperatorMessage(undefined, response.status),
      statusToCode(response.status),
    )
  }

  if (!isJson) {
    // Non-JSON text response (e.g. plain-text errors). T is typed by the caller;
    // the cast is intentional — there is no generic way to prove string ⊆ T.
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
      body: body instanceof FormData ? body : JSON.stringify(body),
    }),

  put: <T>(endpoint: string, body?: unknown, options?: RequestOptions) =>
    request<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: body instanceof FormData ? body : JSON.stringify(body),
    }),

  patch: <T>(endpoint: string, body?: unknown, options?: RequestOptions) =>
    request<T>(endpoint, {
      ...options,
      method: 'PATCH',
      body: body instanceof FormData ? body : JSON.stringify(body),
    }),

  delete: <T>(endpoint: string, options?: RequestOptions) =>
    request<T>(endpoint, { ...options, method: 'DELETE' }),
}

/** Operator-safe message: never dump HTML or multi-kilobyte bodies into the UI. */
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
