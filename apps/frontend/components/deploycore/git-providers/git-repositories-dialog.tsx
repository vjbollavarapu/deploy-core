'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { Archive, GitBranch, Globe, Lock, RefreshCw, FolderGit2 } from 'lucide-react'
import { toast } from 'sonner'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { LoadingState } from '@/components/platform/loading-state'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { ApiError } from '@/lib/api'
import { fetchGitRepositories, syncGitConnection, type WireGitRepository } from '@/lib/integrations'
import {
  applySyncResult,
  collectRepositoryPages,
  isGitHubAppConnection,
  repositoriesToDisplayAfterSync,
  repositoryVisibility,
  safeRepositoryHref,
  suspendedSyncStatus,
} from '@/lib/github/providers'
import type { GitProviderConnection } from '@/lib/types'

interface GitRepositoriesDialogProps {
  connection: GitProviderConnection | null
  open: boolean
  onOpenChange: (open: boolean) => void
  canSync?: boolean
  onSyncComplete?: () => void
  onRepositoryCount?: (connectionId: string, count: number) => void
  onSuspended?: (connection: GitProviderConnection) => void
}

export function GitRepositoriesDialog({
  connection,
  open,
  onOpenChange,
  canSync = true,
  onSyncComplete,
  onRepositoryCount,
  onSuspended,
}: GitRepositoriesDialogProps) {
  const [repositories, setRepositories] = useState<WireGitRepository[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const listGeneration = useRef(0)

  const loadRepos = useCallback(async (connId: string) => {
    try {
      setLoading(true)
      setError(null)
      const items = await collectRepositoryPages((page) => fetchGitRepositories(connId, page))
      setRepositories(items)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch repositories')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!open || !connection?.id) return
    let cancelled = false
    const connectionId = connection.id
    async function load() {
      await Promise.resolve()
      if (cancelled) return
      setLoading(true)
      setError(null)
      setRepositories([])
      try {
        const items = await collectRepositoryPages((page) => fetchGitRepositories(connectionId, page))
        if (!cancelled) setRepositories(items)
      } catch (err: unknown) {
        if (cancelled) return
        setRepositories([])
        setError(err instanceof Error ? err.message : 'Failed to fetch repositories')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
      listGeneration.current += 1
    }
  }, [open, connection?.id])

  const handleSync = async () => {
    if (!connection || !canSync || syncing) return
    const previous = repositories
    const generation = listGeneration.current
    const connectionId = connection.id
    const stillCurrent = () => listGeneration.current === generation
    try {
      setSyncing(true)
      const result = await syncGitConnection(connectionId)
      if (!stillCurrent()) return
      const updated = applySyncResult(result)
      if (updated.providerStatus === 'disabled') {
        onSuspended?.(connection)
        toast.error('GitHub installation is suspended', {
          description: 'Repository sync did not run.',
        })
        setRepositories(previous)
        return
      }
      const displayed = await repositoriesToDisplayAfterSync({
        previous,
        syncRepositories: result.repositories,
        loadPages: (page) => fetchGitRepositories(connectionId, page),
      })
      if (!stillCurrent()) return
      if (isGitHubAppConnection(connection) && updated.repositoryCountKnown) {
        onRepositoryCount?.(connection.id, updated.repositoryCount)
      }
      if (displayed.preservedPrevious) {
        setRepositories(previous)
        const reloadError = displayed.reloadError
        toast.error('Repositories were synchronized, but the list could not be reloaded', {
          description:
            reloadError instanceof Error
              ? reloadError.message
              : 'The previous repository list is unchanged.',
        })
      } else {
        setRepositories([...displayed.repositories])
        toast.success(`Repositories synchronized for ${connection.account}`)
      }
      onSyncComplete?.()
    } catch (err) {
      if (!stillCurrent()) return
      setRepositories(previous)
      const suspended = err instanceof ApiError ? suspendedSyncStatus(err) : null
      if (suspended) onSuspended?.(connection)
      toast.error(suspended ? 'GitHub installation is suspended' : 'Failed to synchronize repositories', {
        description: err instanceof ApiError ? err.message : 'Previously loaded repositories are unchanged.',
      })
    } finally {
      setSyncing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <div>
            <DialogTitle>{connection?.account} repositories</DialogTitle>
            <DialogDescription>
              Synchronized repositories available for application source definitions.
            </DialogDescription>
          </div>
          {canSync ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void handleSync()}
              disabled={syncing || loading}
              aria-busy={syncing}
              className="shrink-0 gap-1.5"
            >
              <RefreshCw className={`size-3.5 ${syncing ? 'animate-spin' : ''}`} />
              {syncing ? 'Syncing…' : 'Sync now'}
            </Button>
          ) : null}
        </DialogHeader>

        <div className="max-h-[500px] min-h-[250px] flex-1 overflow-y-auto pr-1">
          {loading ? (
            <div className="py-12">
              <LoadingState label="Fetching synchronized repositories…" />
            </div>
          ) : error ? (
            <ErrorState
              title="Failed to load repositories"
              message={error}
              onRetry={() => connection?.id && void loadRepos(connection.id)}
            />
          ) : repositories.length === 0 ? (
            <EmptyState
              icon={FolderGit2}
              title="No repositories found"
              description="No repositories are synchronized yet. Sync repositories to pull them from the Git provider."
            />
          ) : (
            <div className="divide-y divide-border rounded-md border border-border">
              {repositories.map((repo) => {
                const visibility = repositoryVisibility(repo.metadata)
                const href = safeRepositoryHref(repo.htmlUrl)
                return (
                  <div key={repo.id} className="flex flex-col gap-1.5 p-3">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-2">
                        {href ? (
                          <a
                            href={href}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="truncate font-mono text-sm font-medium text-foreground underline-offset-4 hover:underline"
                          >
                            {repo.fullName}
                            <span className="sr-only"> opens in a new tab</span>
                          </a>
                        ) : (
                          <span className="truncate font-mono text-sm font-medium text-foreground">{repo.fullName}</span>
                        )}
                        {visibility.visibility === 'Private' ? (
                          <Badge variant="outline" className="shrink-0 gap-1 text-[10px]">
                            <Lock className="size-2.5" /> Private
                          </Badge>
                        ) : null}
                        {visibility.visibility === 'Public' ? (
                          <Badge variant="secondary" className="shrink-0 gap-1 text-[10px]">
                            <Globe className="size-2.5" /> Public
                          </Badge>
                        ) : null}
                        {visibility.archived ? (
                          <Badge variant="outline" className="shrink-0 gap-1 text-[10px]">
                            <Archive className="size-2.5" /> Archived
                          </Badge>
                        ) : null}
                      </div>
                      <div className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                        <GitBranch className="size-3" />
                        <span className="font-mono">{repo.defaultBranch || '—'}</span>
                      </div>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      <span className="font-mono break-all">{displayCloneUrl(repo.cloneUrl)}</span>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function displayCloneUrl(cloneUrl: string): string {
  return safeRepositoryHref(cloneUrl) ?? 'Clone URL unavailable'
}
