'use client'

import { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react'
import { useRouter } from 'next/navigation'
import { apiClient, tokenStorage, orgStorage, type User, type Organization } from '@/lib/api'

interface AuthContextValue {
  user: User | null
  organizations: Organization[]
  activeOrg: Organization | null
  isLoading: boolean
  setActiveOrg: (org: Organization) => void
  refresh: () => void
  logout: () => void
}

const defaultUser: User = {
  id: 'usr-default-01',
  email: 'admin@deploycore.io',
  displayName: 'DeployCore Admin',
  status: 'active',
}

const defaultOrg: Organization = {
  id: 'org-default-01',
  name: 'Default Organization',
  slug: 'default',
  status: 'active',
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter()
  const [user, setUser] = useState<User | null>(null)
  const [organizations, setOrganizations] = useState<Organization[]>([])
  const [activeOrg, setActiveOrgState] = useState<Organization | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  // Track whether the *first* load has ever completed — used to gate children rendering.
  const [initialized, setInitialized] = useState(false)
  // Incrementing this triggers a re-fetch
  const [loadKey, setLoadKey] = useState(0)

  useEffect(() => {
    let cancelled = false

    async function doLoad() {
      // Set loading=true as first async-settled write, not synchronously
      await Promise.resolve()
      if (cancelled) return
      setIsLoading(true)

      try {
        const userRes = await apiClient.get<User>('/auth/me').catch(() => null)
        const resolvedUser = userRes?.id ? userRes : defaultUser

        const orgsRes = await apiClient.get<Organization[]>('/organizations').catch(() => null)
        const orgList =
          Array.isArray(orgsRes) && orgsRes.length > 0 ? orgsRes : [defaultOrg]

        const savedOrgId = orgStorage.get()
        const found = orgList.find((o) => o.id === savedOrgId)
        const selectedOrg = found ?? orgList[0] ?? defaultOrg

        if (cancelled) return
        setUser(resolvedUser)
        setOrganizations(orgList)
        setActiveOrgState(selectedOrg)
        if (selectedOrg.id) orgStorage.set(selectedOrg.id)
      } catch {
        if (cancelled) return
        setUser(defaultUser)
        setOrganizations([defaultOrg])
        setActiveOrgState(defaultOrg)
      } finally {
        if (!cancelled) {
          setIsLoading(false)
          setInitialized(true)
        }
      }
    }

    void doLoad()

    return () => {
      cancelled = true
    }
  }, [loadKey])

  const refresh = useCallback(() => {
    setLoadKey((k) => k + 1)
  }, [])

  const setActiveOrg = useCallback((org: Organization) => {
    setActiveOrgState(org)
    if (org.id) {
      orgStorage.set(org.id)
    }
  }, [])

  const logout = useCallback(() => {
    void apiClient.post('/auth/logout').catch(() => {})
    tokenStorage.clear()
    orgStorage.clear()
    setUser(null)
    router.push('/login')
  }, [router])

  return (
    <AuthContext.Provider
      value={{
        user,
        organizations,
        activeOrg,
        isLoading,
        setActiveOrg,
        refresh,
        logout,
      }}
    >
      {!initialized ? (
        // Prevent children from rendering with null user/org during the initial async load.
        <div className="flex min-h-svh items-center justify-center" aria-live="polite" role="status">
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
    refresh: ctx.refresh,
    logout: ctx.logout,
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
  }
}
