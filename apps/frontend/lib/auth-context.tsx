'use client'

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { DeployCoreLogo } from '@/components/platform/deploycore-logo'
import {
  apiClient,
  ApiError,
  clearSessionTokens,
  orgStorage,
  persistTokenPair,
  refreshTokenStorage,
  tokenStorage,
  type AuthResult,
  type LoginRequest,
  type Organization,
  type Page,
  type RegisterRequest,
  type User,
} from '@/lib/api'

const AUTH_PUBLIC_PATHS = ['/login', '/register', '/forgot-password', '/reset-password'] as const

function isAuthPublicPath(pathname: string | null): boolean {
  if (!pathname) return false
  return AUTH_PUBLIC_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`))
}

type MeResponse = { user?: User }
type OrgCreateResponse = { organization?: Organization }

interface AuthContextValue {
  user: User | null
  organizations: Organization[]
  activeOrg: Organization | null
  /** True while the initial session validation is in progress. */
  isLoading: boolean
  /** True once the first session check has finished (success or unauthenticated). */
  isAuthenticated: boolean
  setActiveOrg: (org: Organization) => void
  refresh: () => void
  login: (input: LoginRequest) => Promise<void>
  register: (input: RegisterRequest) => Promise<void>
  logout: () => Promise<void>
  createOrganization: (name: string, slug: string) => Promise<Organization>
}

const AuthContext = createContext<AuthContextValue | null>(null)

async function fetchMe(): Promise<User | null> {
  if (!tokenStorage.get()) return null
  try {
    const res = await apiClient.get<MeResponse>('/auth/me')
    return res.user?.id ? res.user : null
  } catch (err) {
    if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
      return null
    }
    throw err
  }
}

async function fetchOrganizations(): Promise<Organization[]> {
  const res = await apiClient.get<Page<Organization>>('/organizations?limit=100')
  return (res.items ?? []).filter((o) => Boolean(o.id))
}

function selectActiveOrg(orgList: Organization[]): Organization | null {
  if (orgList.length === 0) return null
  const savedOrgId = orgStorage.get()
  const found = savedOrgId ? orgList.find((o) => o.id === savedOrgId) : undefined
  return found ?? orgList[0] ?? null
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()
  const [user, setUser] = useState<User | null>(null)
  const [organizations, setOrganizations] = useState<Organization[]>([])
  const [activeOrg, setActiveOrgState] = useState<Organization | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [initialized, setInitialized] = useState(false)
  const [loadKey, setLoadKey] = useState(0)

  const applySession = useCallback(async (nextUser: User) => {
    setUser(nextUser)
    try {
      const orgList = await fetchOrganizations()
      const selected = selectActiveOrg(orgList)
      setOrganizations(orgList)
      setActiveOrgState(selected)
      if (selected?.id) {
        orgStorage.set(selected.id)
      } else {
        orgStorage.clear()
      }
    } catch {
      // Authenticated user with no readable org membership / API failure — do not fabricate an org.
      setOrganizations([])
      setActiveOrgState(null)
      orgStorage.clear()
    }
  }, [])

  const clearLocalSession = useCallback(() => {
    clearSessionTokens()
    orgStorage.clear()
    setUser(null)
    setOrganizations([])
    setActiveOrgState(null)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function bootstrap() {
      await Promise.resolve()
      if (cancelled) return
      setIsLoading(true)

      try {
        if (!tokenStorage.get() && !refreshTokenStorage.get()) {
          if (!cancelled) clearLocalSession()
          return
        }

        let me = await fetchMe()
        if (!me && refreshTokenStorage.get()) {
          // Access token missing/expired; apiClient refresh may already have run inside fetchMe.
          me = await fetchMe()
        }

        if (cancelled) return

        if (!me) {
          clearLocalSession()
          return
        }

        await applySession(me)
      } catch {
        if (!cancelled) clearLocalSession()
      } finally {
        if (!cancelled) {
          setIsLoading(false)
          setInitialized(true)
        }
      }
    }

    void bootstrap()
    return () => {
      cancelled = true
    }
  }, [loadKey, applySession, clearLocalSession])

  // Route protection — wait until initialized to avoid redirect loops / fake identity flash.
  useEffect(() => {
    if (!initialized || isLoading) return
    const onPublic = isAuthPublicPath(pathname)

    if (!user && !onPublic) {
      const next = pathname && pathname !== '/' ? `?next=${encodeURIComponent(pathname)}` : ''
      router.replace(`/login${next}`)
      return
    }

    if (user && onPublic) {
      router.replace('/dashboard')
    }
  }, [initialized, isLoading, user, pathname, router])

  const refresh = useCallback(() => {
    setLoadKey((k) => k + 1)
  }, [])

  const setActiveOrg = useCallback((org: Organization) => {
    setActiveOrgState(org)
    if (org.id) orgStorage.set(org.id)
  }, [])

  const login = useCallback(
    async (input: LoginRequest) => {
      const result = await apiClient.post<AuthResult>('/auth/login', input, { skipAuth: true })
      if (!result.tokens?.accessToken || !result.user?.id) {
        throw new ApiError(500, 'Login response did not include credentials.', 'INTERNAL_ERROR')
      }
      persistTokenPair(result.tokens)
      await applySession(result.user)
      setInitialized(true)
      setIsLoading(false)
    },
    [applySession],
  )

  const register = useCallback(
    async (input: RegisterRequest) => {
      const result = await apiClient.post<AuthResult>('/auth/register', input, { skipAuth: true })
      if (!result.tokens?.accessToken || !result.user?.id) {
        throw new ApiError(500, 'Registration response did not include credentials.', 'INTERNAL_ERROR')
      }
      persistTokenPair(result.tokens)
      await applySession(result.user)
      setInitialized(true)
      setIsLoading(false)
    },
    [applySession],
  )

  const logout = useCallback(async () => {
    const refreshToken = refreshTokenStorage.get()
    try {
      await apiClient.post('/auth/logout', refreshToken ? { refreshToken } : {})
    } catch {
      // Still clear local session; do not leave the browser pretending to be authenticated.
    }
    clearLocalSession()
    router.replace('/login')
  }, [clearLocalSession, router])

  const createOrganization = useCallback(async (name: string, slug: string) => {
    const res = await apiClient.post<OrgCreateResponse>('/organizations', { name, slug })
    const org = res.organization
    if (!org?.id) {
      throw new ApiError(500, 'Organization create response was incomplete.', 'INTERNAL_ERROR')
    }
    setOrganizations((prev) => {
      const next = [...prev.filter((o) => o.id !== org.id), org]
      return next
    })
    setActiveOrgState(org)
    orgStorage.set(org.id)
    return org
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      organizations,
      activeOrg,
      isLoading: !initialized || isLoading,
      isAuthenticated: Boolean(user),
      setActiveOrg,
      refresh,
      login,
      register,
      logout,
      createOrganization,
    }),
    [
      user,
      organizations,
      activeOrg,
      initialized,
      isLoading,
      setActiveOrg,
      refresh,
      login,
      register,
      logout,
      createOrganization,
    ],
  )

  const onPublic = isAuthPublicPath(pathname)
  const showBootSplash = !initialized || (isLoading && !onPublic)
  const blockingRedirect =
    initialized &&
    !isLoading &&
    (( !user && !onPublic) || (user && onPublic))

  return (
    <AuthContext.Provider value={value}>
      {showBootSplash || blockingRedirect ? (
        <div
          className="flex min-h-svh flex-col items-center justify-center gap-3 bg-background"
          aria-live="polite"
          role="status"
        >
          <DeployCoreLogo size={36} className="rounded-md" />
          <span className="sr-only">Loading DeployCore…</span>
        </div>
      ) : (
        children
      )}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return {
    user: ctx.user,
    isLoading: ctx.isLoading,
    isAuthenticated: ctx.isAuthenticated,
    refresh: ctx.refresh,
    login: ctx.login,
    register: ctx.register,
    logout: ctx.logout,
    createOrganization: ctx.createOrganization,
  }
}

export function useOrganization() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useOrganization must be used within an AuthProvider')
  }
  return {
    organizations: ctx.organizations,
    activeOrg: ctx.activeOrg,
    isLoading: ctx.isLoading,
    setActiveOrg: ctx.setActiveOrg,
    refresh: ctx.refresh,
    createOrganization: ctx.createOrganization,
  }
}
