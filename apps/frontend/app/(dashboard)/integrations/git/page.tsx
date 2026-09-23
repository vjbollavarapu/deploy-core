'use client'

import { useCallback, useState } from 'react'
import { GitBranch, Plus } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { EmptyState } from '@/components/platform/empty-state'
import { GitProvidersTable } from '@/components/deploycore/git-providers/git-providers-table'
import { ConnectGitProviderDialog } from '@/components/deploycore/git-providers/connect-git-provider-dialog'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import {
  fetchGitConnections,
  GIT_PROVIDER_TYPES,
  mapWireGitConnection,
} from '@/lib/integrations'
import { gitProviders as rawGitProviders } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { GitProviderConnection } from '@/lib/types'

export default function GitProvidersPage() {
  const { activeOrg } = useOrganization()
  const [connectOpen, setConnectOpen] = useState(false)

  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  const loadGitProviders = useCallback(async (): Promise<GitProviderConnection[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawGitProviders) : []
    try {
      const res = await fetchGitConnections(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map((conn) => mapWireGitConnection(conn))
      }
      return isDemo ? getDemoFixtures(rawGitProviders) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawGitProviders)
      throw err
    }
  }, [orgId, isDemo])

  const { data, isLoading, error, reload } = useApiQuery(loadGitProviders)
  const providers = data || []

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Git Providers"
        description="Source control connections for GitHub, GitLab, Bitbucket, and generic Git remotes."
        actions={
          <Button
            size="sm"
            className="gap-1.5"
            onClick={() => setConnectOpen(true)}
          >
            <Plus className="size-4" />
            Connect Git Provider
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Connected Providers</CardTitle>
          <CardDescription>
            Supported hosts: {GIT_PROVIDER_TYPES.join(', ')}. Credentials are sealed with AES-256-GCM.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="py-12">
              <LoadingState label="Loading Git provider connections…" />
            </div>
          ) : error && !isDemo ? (
            <div className="p-6">
              <ErrorState
                title="Failed to load Git connections"
                message={error}
                onRetry={reload}
              />
            </div>
          ) : providers.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={GitBranch}
                title="No Git providers connected"
                description="Connect GitHub, GitLab, Bitbucket, or a Generic Git host to synchronize repositories and automate deployments."
                action={
                  <Button size="sm" onClick={() => setConnectOpen(true)} className="gap-1.5">
                    <Plus className="size-4" />
                    Connect Git Provider
                  </Button>
                }
              />
            </div>
          ) : (
            <GitProvidersTable
              providers={providers}
              onConnectionChange={reload}
            />
          )}
        </CardContent>
      </Card>

      <ConnectGitProviderDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        organizationId={orgId}
        onSuccess={reload}
      />
    </PageContainer>
  )
}
