'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { GitBranch } from 'lucide-react'
import { toast } from 'sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { EmptyState } from '@/components/platform/empty-state'
import { GitHubConnectPanel } from '@/components/deploycore/git-providers/github-connect-panel'
import { GitProvidersTable } from '@/components/deploycore/git-providers/git-providers-table'
import { ConnectGitProviderDialog } from '@/components/deploycore/git-providers/connect-git-provider-dialog'
import { useAuth, useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import { ApiError, apiClient, type Page } from '@/lib/api'
import type { WireMember } from '@/lib/rbac'
import {
  beginGitHubInstallation,
  fetchGitConnections,
  fetchGitHubAppStatus,
  fetchGitRepositories,
  GIT_PROVIDER_TYPES,
  mapWireGitConnection,
} from '@/lib/integrations'
import {
  accessForOrganizationUser,
  countsForOrganization,
  GIT_ACCESS_UNKNOWN,
  gitHubAppStatusKind,
  installationNavigationTarget,
  patchConnectionStatus,
  repositoryTotalCount,
  REPOSITORY_COUNT_LIMIT,
  shouldBeginInstallation,
  type GitConnectionAccess,
  type RepositoryCountValue,
} from '@/lib/github/providers'
import { gitProviders as rawGitProviders } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { GitProviderConnection } from '@/lib/types'

interface GitProviderList {
  organizationId: string
  connections: GitProviderConnection[]
}

interface CountSnapshot {
  organizationId: string
  counts: Record<string, RepositoryCountValue>
}

export default function GitProvidersPage() {
  const { user } = useAuth()
  const { activeOrg } = useOrganization()
  const [connectOpen, setConnectOpen] = useState(false)
  const [connectRequest, setConnectRequest] = useState<{ organizationId: string } | null>(null)
  const [resolvedAccess, setResolvedAccess] = useState<{
    organizationId: string
    userId: string
    access: GitConnectionAccess
  } | null>(null)
  const [counts, setCounts] = useState<CountSnapshot | null>(null)
  const [statusPatches, setStatusPatches] = useState<{
    organizationId: string
    byId: Record<string, GitProviderConnection>
  }>({ organizationId: '', byId: {} })
  const orgId = activeOrg?.id || ''
  const orgRef = useRef(orgId)
  const isDemo = isDemoModeEnabled()
  const connecting = connectRequest?.organizationId === orgId

  const loadGitProviders = useCallback(async (): Promise<GitProviderList> => {
    if (!orgId) {
      return { organizationId: orgId, connections: isDemo ? getDemoFixtures(rawGitProviders) : [] }
    }
    try {
      const res = await fetchGitConnections(orgId)
      if (res?.items && res.items.length > 0) {
        return {
          organizationId: orgId,
          connections: res.items.map((conn) => mapWireGitConnection(conn)),
        }
      }
      return { organizationId: orgId, connections: isDemo ? getDemoFixtures(rawGitProviders) : [] }
    } catch (err) {
      if (isDemo) return { organizationId: orgId, connections: getDemoFixtures(rawGitProviders) }
      throw err
    }
  }, [orgId, isDemo])

  const loadAppStatus = useCallback(async () => fetchGitHubAppStatus(), [])

  const connectionsQuery = useApiQuery(loadGitProviders)
  const { reload: reloadConnections } = connectionsQuery
  const statusQuery = useApiQuery(loadAppStatus)
  const seenOrg = useRef<string | null>(null)

  useEffect(() => {
    orgRef.current = orgId
  }, [orgId])

  useEffect(() => {
    if (seenOrg.current === null) {
      seenOrg.current = orgId
      return
    }
    if (seenOrg.current === orgId) return
    seenOrg.current = orgId
    reloadConnections()
  }, [orgId, reloadConnections])

  useEffect(() => {
    if (!orgId || !user?.id || isDemo) return
    let cancelled = false
    const requestedOrg = orgId
    const requestedUser = user.id
    void accessForOrganizationUser(requestedUser, (offset) =>
      apiClient.get<Page<WireMember>>(`/organizations/${requestedOrg}/members?limit=100&offset=${offset}`, {
        orgId: requestedOrg,
      }),
    )
      .then((next) => {
        if (!cancelled) {
          setResolvedAccess({ organizationId: requestedOrg, userId: requestedUser, access: next })
        }
      })
      .catch(() => {
        if (!cancelled) {
          setResolvedAccess({
            organizationId: requestedOrg,
            userId: requestedUser,
            access: GIT_ACCESS_UNKNOWN,
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [orgId, user?.id, isDemo])

  const access =
    !orgId || !user?.id || isDemo
      ? GIT_ACCESS_UNKNOWN
      : resolvedAccess?.organizationId === orgId && resolvedAccess.userId === user.id
        ? resolvedAccess.access
        : GIT_ACCESS_UNKNOWN
  const patches = statusPatches.organizationId === orgId ? statusPatches.byId : {}
  const currentList =
    connectionsQuery.data && connectionsQuery.data.organizationId === orgId
      ? connectionsQuery.data
      : null
  const providers = (currentList?.connections ?? []).map(
    (connection) => patches[connection.id] ?? connection,
  )
  const visibleCounts = countsForOrganization(orgId, counts)

  useEffect(() => {
    if (!currentList) return
    const org = currentList.organizationId
    const apps = currentList.connections.filter((connection) => connection.authMode === 'github_app')
    let cancelled = false
    void Promise.all(
      apps.map(async (connection) => {
        try {
          const page = await fetchGitRepositories(connection.id, {
            limit: REPOSITORY_COUNT_LIMIT,
            offset: 0,
          })
          const total = repositoryTotalCount(page)
          return [connection.id, total == null ? 'unavailable' : total] as const
        } catch {
          return [connection.id, 'unavailable'] as const
        }
      }),
    ).then((entries) => {
      if (cancelled) return
      setCounts({ organizationId: org, counts: Object.fromEntries(entries) })
    })
    return () => {
      cancelled = true
    }
  }, [currentList])

  const statusKind = gitHubAppStatusKind({
    loading: statusQuery.isLoading,
    error: statusQuery.error,
    configured: statusQuery.data ? statusQuery.data.configured === true : null,
  })
  const canManage = !access.known || access.canManage
  const canRead = !access.known || access.canRead
  const orgMismatch = Boolean(connectionsQuery.data && connectionsQuery.data.organizationId !== orgId)

  async function connectGitHub() {
    if (
      !shouldBeginInstallation({
        pending: connecting,
        organizationId: orgId,
        configured: statusKind === 'configured',
        canManage,
      })
    ) {
      return
    }
    const requestedOrg = orgId
    setConnectRequest({ organizationId: requestedOrg })
    try {
      const result = await beginGitHubInstallation(requestedOrg)
      if (orgRef.current !== requestedOrg) return
      const target = installationNavigationTarget(result.installationUrl)
      if (!target) {
        throw new Error('GitHub did not return an installation link.')
      }
      window.location.assign(target)
    } catch (err) {
      if (orgRef.current !== requestedOrg) return
      const message = err instanceof ApiError ? err.message : 'GitHub installation could not be started.'
      toast.error('Could not connect GitHub', { description: message })
      setConnectRequest(null)
    }
  }

  function noteRepositoryCount(connectionId: string, count: number) {
    setCounts((current) => {
      if (!current || current.organizationId !== orgId) {
        return { organizationId: orgId, counts: { [connectionId]: count } }
      }
      return {
        organizationId: orgId,
        counts: { ...current.counts, [connectionId]: count },
      }
    })
  }

  function noteSuspended(connection: GitProviderConnection) {
    setStatusPatches((current) => {
      const byId = current.organizationId === orgId ? current.byId : {}
      return {
        organizationId: orgId,
        byId: { ...byId, [connection.id]: patchConnectionStatus(connection, 'disabled') },
      }
    })
  }

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Git Providers"
        description="Source control connections for GitHub, GitLab, Bitbucket, and generic Git remotes."
      />

      <GitHubConnectPanel
        statusKind={statusKind}
        statusError={statusQuery.error}
        canManage={canManage}
        connecting={connecting}
        onConnect={() => void connectGitHub()}
        onAdvanced={() => setConnectOpen(true)}
        onRetryStatus={statusQuery.reload}
      />

      <Card>
        <CardHeader>
          <CardTitle>Connected providers</CardTitle>
          <CardDescription>
            Supported hosts: {GIT_PROVIDER_TYPES.join(', ')}. Credentials stay sealed and are not shown here.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {(connectionsQuery.isLoading && !currentList) || orgMismatch ? (
            <div className="py-12">
              <LoadingState label="Loading Git provider connections…" />
            </div>
          ) : connectionsQuery.error && !currentList && !isDemo ? (
            <div className="p-6">
              <ErrorState
                title="Failed to load Git connections"
                message={connectionsQuery.error}
                onRetry={connectionsQuery.reload}
              />
            </div>
          ) : providers.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={GitBranch}
                title="No Git providers connected"
                description="Connect GitHub with the DeployCore GitHub App, or use a personal access token for GitHub, GitLab, Bitbucket, or a generic Git host."
                action={
                  canManage ? (
                    <Button size="sm" variant="outline" onClick={() => setConnectOpen(true)}>
                      Connect using token
                    </Button>
                  ) : undefined
                }
              />
            </div>
          ) : (
            <GitProvidersTable
              providers={providers}
              canManage={canManage}
              canRead={canRead}
              repositoryCounts={visibleCounts}
              onConnectionChange={() => {
                setStatusPatches({ organizationId: orgId, byId: {} })
                reloadConnections()
              }}
              onRepositoryCount={noteRepositoryCount}
              onSuspended={noteSuspended}
            />
          )}
        </CardContent>
      </Card>

      <ConnectGitProviderDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        organizationId={orgId}
        onSuccess={connectionsQuery.reload}
      />
    </PageContainer>
  )
}
