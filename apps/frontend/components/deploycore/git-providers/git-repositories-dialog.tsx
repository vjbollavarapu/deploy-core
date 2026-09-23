'use client'

import { useCallback, useEffect, useState } from 'react'
import { GitBranch, Globe, Lock, RefreshCw, FolderGit2 } from 'lucide-react'
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
import { fetchGitRepositories, syncGitConnection, type WireGitRepository } from '@/lib/integrations'
import type { GitProviderConnection } from '@/lib/types'

interface GitRepositoriesDialogProps {
  connection: GitProviderConnection | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onSyncComplete?: () => void
}

export function GitRepositoriesDialog({
  connection,
  open,
  onOpenChange,
  onSyncComplete,
}: GitRepositoriesDialogProps) {
  const [repositories, setRepositories] = useState<WireGitRepository[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)

  const loadRepos = useCallback(async (connId: string) => {
    try {
      setLoading(true)
      setError(null)
      const res = await fetchGitRepositories(connId)
      setRepositories(res?.items || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch repositories')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    if (!open || !connection?.id) {
      return
    }

    fetchGitRepositories(connection.id)
      .then((res) => {
        if (cancelled) return
        setRepositories(res?.items || [])
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Failed to fetch repositories')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [open, connection?.id])

  const handleSync = async () => {
    if (!connection) return
    try {
      setSyncing(true)
      await syncGitConnection(connection.id)
      toast.success(`Repositories synchronized from ${connection.type}`)
      await loadRepos(connection.id)
      onSyncComplete?.()
    } catch (err) {
      toast.error('Failed to synchronize repositories', {
        description: err instanceof Error ? err.message : 'Please check connection credentials.',
      })
    } finally {
      setSyncing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] sm:max-w-2xl flex flex-col">
        <DialogHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <div>
            <DialogTitle>{connection?.account} Repositories</DialogTitle>
            <DialogDescription>
              Synchronized repositories available for application source definitions.
            </DialogDescription>
          </div>
          <Button
            size="sm"
            variant="outline"
            onClick={handleSync}
            disabled={syncing || loading}
            className="gap-1.5 shrink-0"
          >
            <RefreshCw className={`size-3.5 ${syncing ? 'animate-spin' : ''}`} />
            {syncing ? 'Syncing…' : 'Sync Now'}
          </Button>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto min-h-[250px] max-h-[500px] pr-1">
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
              description="No repositories synchronized yet. Click 'Sync Now' to pull repos from your Git provider."
            />
          ) : (
            <div className="divide-y divide-border rounded-md border border-border">
              {repositories.map((repo) => (
                <div
                  key={repo.id}
                  className="flex flex-col gap-1.5 p-3 hover:bg-muted/50 transition-colors"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="font-mono text-sm font-medium text-foreground truncate">
                        {repo.fullName}
                      </span>
                      {repo.metadata?.private ? (
                        <Badge variant="outline" className="text-[10px] gap-1 shrink-0">
                          <Lock className="size-2.5" /> Private
                        </Badge>
                      ) : (
                        <Badge variant="secondary" className="text-[10px] gap-1 shrink-0">
                          <Globe className="size-2.5" /> Public
                        </Badge>
                      )}
                    </div>
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground shrink-0">
                      <GitBranch className="size-3" />
                      <span className="font-mono">{repo.defaultBranch || 'main'}</span>
                    </div>
                  </div>
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span className="font-mono truncate max-w-md">{repo.cloneUrl}</span>
                    <span>
                      {repo.lastSyncAt ? `Synced ${new Date(repo.lastSyncAt).toLocaleTimeString()}` : ''}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
