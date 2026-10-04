import { GitBranch, KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import type { GitHubAppStatusKind } from '@/lib/github/providers'

interface GitHubConnectPanelProps {
  statusKind: GitHubAppStatusKind
  statusError: string | null
  canManage: boolean
  connecting: boolean
  onConnect: () => void
  onAdvanced: () => void
  onRetryStatus: () => void
}

export function GitHubConnectPanel({
  statusKind,
  statusError,
  canManage,
  connecting,
  onConnect,
  onAdvanced,
  onRetryStatus,
}: GitHubConnectPanelProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>GitHub</CardTitle>
        <CardDescription>
          Connect your GitHub account or organization using the DeployCore GitHub App.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        {statusKind === 'loading' ? (
          <LoadingState label="Checking GitHub App configuration…" />
        ) : null}

        {statusKind === 'unavailable' ? (
          <ErrorState
            title="GitHub App status could not be loaded"
            message={
              statusError ||
              'Existing connections are unchanged. This does not mean the GitHub App is unconfigured.'
            }
            onRetry={onRetryStatus}
          />
        ) : null}

        {statusKind === 'unconfigured' ? (
          <p className="text-sm text-muted-foreground" role="status">
            An administrator must configure the GitHub App for this DeployCore installation before
            accounts can be connected with Connect GitHub.
          </p>
        ) : null}

        {statusKind === 'configured' && canManage ? (
          <div>
            <Button
              type="button"
              size="sm"
              className="gap-1.5"
              onClick={onConnect}
              disabled={connecting}
              aria-busy={connecting}
              aria-label={connecting ? 'Connecting to GitHub' : 'Connect GitHub'}
            >
              <GitBranch className="size-4" />
              {connecting ? 'Connecting…' : 'Connect GitHub'}
            </Button>
          </div>
        ) : null}

        {statusKind === 'configured' && !canManage ? (
          <p className="text-sm text-muted-foreground" role="status">
            You can view Git connections. Connecting a GitHub App requires permission to manage Git providers.
          </p>
        ) : null}

        {canManage ? (
          <div className="flex flex-col gap-3 border-t border-border pt-4">
            <div>
              <h3 className="text-sm font-medium text-foreground">Advanced</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Need to use a personal access token or another Git provider?
              </p>
            </div>
            <div>
              <Button type="button" size="sm" variant="outline" className="gap-1.5" onClick={onAdvanced}>
                <KeyRound className="size-4" />
                Connect using token
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
