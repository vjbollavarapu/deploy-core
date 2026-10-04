'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { useOrganization } from '@/lib/auth-context'
import {
  GITHUB_APP_CALLBACK_PATH,
  GIT_PROVIDERS_PATH,
  githubCallbackView,
  matchingOrganization,
  type GitHubCallbackOutcome,
} from '@/lib/github/callback'
import { beginGitHubCallback, pauseGitHubCallbackLifecycle } from '@/lib/github/callback-lifecycle'
import { completeGitHubInstallation, syncGitConnection } from '@/lib/integrations'
import { ApiError } from '@/lib/api'

export default function GitHubInstallationCallbackPage() {
  const router = useRouter()
  const { organizations, setActiveOrg } = useOrganization()
  const [outcome, setOutcome] = useState<GitHubCallbackOutcome | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [syncMessage, setSyncMessage] = useState<string | null>(null)

  useEffect(() => {
    const search = window.location.search
    if (search) {
      window.history.replaceState(window.history.state, '', GITHUB_APP_CALLBACK_PATH)
      router.replace(GITHUB_APP_CALLBACK_PATH, { scroll: false })
    }

    let cancelled = false
    void beginGitHubCallback(search, completeGitHubInstallation).then((next) => {
      if (cancelled) return
      setOutcome(next)
      if (next.kind !== 'SUCCESS') return
      const org = matchingOrganization(next.connection.organizationId, organizations)
      if (org?.id) setActiveOrg(org)
    })
    return () => {
      cancelled = true
      pauseGitHubCallbackLifecycle()
    }
  }, [organizations, router, setActiveOrg])

  async function retrySync(connectionId: string) {
    setSyncing(true)
    setSyncMessage(null)
    try {
      const result = await syncGitConnection(connectionId)
      setOutcome({
        kind: 'SUCCESS',
        connection: result.connection,
        repositoryCount: result.repositories.length,
      })
      const org = matchingOrganization(result.connection.organizationId, organizations)
      if (org?.id) setActiveOrg(org)
    } catch (error) {
      setSyncMessage(error instanceof ApiError ? error.message : 'Repository sync failed. Return to Git Providers and try again.')
    } finally {
      setSyncing(false)
    }
  }

  if (!outcome) {
    return (
      <PageContainer>
        <PageHeader title="Connecting GitHub…" description="Finishing the GitHub App installation." />
        <LoadingState label="Connecting GitHub…" />
      </PageContainer>
    )
  }

  const view = githubCallbackView(outcome)
  const retryConnectionId = view.retryConnectionId
  const actions = (
    <div className="flex flex-wrap items-center gap-2">
      {retryConnectionId ? (
        <Button type="button" size="sm" disabled={syncing} onClick={() => void retrySync(retryConnectionId)}>
          {syncing ? 'Syncing repositories…' : 'Retry sync'}
        </Button>
      ) : null}
      {view.showTryAgain ? (
        <Button size="sm" render={<Link href={GIT_PROVIDERS_PATH} />}>
          Try again
        </Button>
      ) : null}
      {view.showReturn ? (
        <Button size="sm" variant="outline" render={<Link href={GIT_PROVIDERS_PATH} />}>
          Return to Git Providers
        </Button>
      ) : null}
    </div>
  )

  return (
    <PageContainer>
      <PageHeader title={view.title} description={view.message} />
      {view.tone === 'success' ? (
        <div className="flex flex-col gap-4" role="status" aria-live="polite">
          {actions}
        </div>
      ) : (
        <ErrorState title={view.title} message={syncMessage ?? view.message} action={actions} />
      )}
    </PageContainer>
  )
}
