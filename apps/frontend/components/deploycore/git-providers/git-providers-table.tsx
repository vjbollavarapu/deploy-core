'use client'

import { useState } from 'react'
import { ExternalLink, FolderGit2, MoreHorizontal, RefreshCw, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ApiError } from '@/lib/api'
import { deleteGitConnection, syncGitConnection } from '@/lib/integrations'
import {
  accountTypeLabel,
  applySyncResult,
  authModeLabel,
  connectionStatusView,
  disconnectCopy,
  githubManageUrl,
  isGitHubAppConnection,
  repositorySelectionLabel,
  suspendedSyncStatus,
  type RepositoryCountValue,
} from '@/lib/github/providers'
import { TONE_CLASSES } from '@/lib/status'
import { cn } from '@/lib/utils'
import type { GitProviderConnection } from '@/lib/types'
import { GitRepositoriesDialog } from './git-repositories-dialog'

interface GitProvidersTableProps {
  providers: GitProviderConnection[]
  canManage: boolean
  canRead: boolean
  repositoryCounts: Record<string, RepositoryCountValue> | null
  onConnectionChange?: () => void
  onRepositoryCount?: (connectionId: string, count: number) => void
  onSuspended?: (connection: GitProviderConnection) => void
}

export function GitProvidersTable({
  providers,
  canManage,
  canRead,
  repositoryCounts,
  onConnectionChange,
  onRepositoryCount,
  onSuspended,
}: GitProvidersTableProps) {
  const [activeRepoConn, setActiveRepoConn] = useState<GitProviderConnection | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<GitProviderConnection | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [syncingId, setSyncingId] = useState<string | null>(null)
  const deleteCopy = deleteTarget ? disconnectCopy(deleteTarget) : null

  const handleSync = async (provider: GitProviderConnection) => {
    if (!canManage || syncingId) return
    try {
      setSyncingId(provider.id)
      const result = await syncGitConnection(provider.id)
      const updated = applySyncResult(result)
      if (updated.providerStatus === 'disabled') {
        onSuspended?.(provider)
        toast.error('GitHub installation is suspended', {
          description: 'Repository sync did not run. Unsuspend the installation on GitHub, then sync again.',
        })
        return
      }
      if (isGitHubAppConnection(provider) && updated.repositoryCountKnown) {
        onRepositoryCount?.(provider.id, updated.repositoryCount)
      }
      toast.success(`Repositories synchronized for ${provider.account}`)
      onConnectionChange?.()
    } catch (err) {
      const suspended = err instanceof ApiError ? suspendedSyncStatus(err) : null
      if (suspended) onSuspended?.(provider)
      toast.error(suspended ? 'GitHub installation is suspended' : 'Repository sync failed', {
        description:
          err instanceof ApiError
            ? err.message
            : 'The connection is unchanged. Try again after checking the provider.',
      })
    } finally {
      setSyncingId(null)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget || !canManage) return
    try {
      setIsDeleting(true)
      await deleteGitConnection(deleteTarget.id)
      toast.success(`Disconnected ${deleteTarget.account}`)
      setDeleteTarget(null)
      onConnectionChange?.()
    } catch (err) {
      toast.error('Failed to disconnect Git provider', {
        description: err instanceof ApiError ? err.message : 'The connection is still in place.',
      })
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Provider</TableHead>
              <TableHead>Account</TableHead>
              <TableHead>Auth</TableHead>
              <TableHead>Access</TableHead>
              <TableHead>Repositories</TableHead>
              <TableHead>Last sync</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-28 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {providers.map((provider) => {
              const isSyncing = syncingId === provider.id
              const manageUrl = githubManageUrl({
                authMode: provider.authMode,
                installationId: provider.installationId,
                accountType: provider.accountType,
                accountLogin: provider.account,
              })
              const showView = canRead
              const showManageActions = canManage
              const hasMenu = showView || showManageActions || Boolean(manageUrl)
              return (
                <TableRow key={provider.id}>
                  <TableCell className="font-medium text-foreground">{provider.type}</TableCell>
                  <TableCell className="text-sm text-foreground">
                    <div className="flex flex-col">
                      <span>{provider.account}</span>
                      {provider.displayName && provider.displayName !== provider.account ? (
                        <span className="text-xs text-muted-foreground">{provider.displayName}</span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {authModeLabel(provider.authMode) ?? '—'}
                  </TableCell>
                  <TableCell>{accessCell(provider)}</TableCell>
                  <TableCell className="text-foreground">
                    <RepositoryCount
                      provider={provider}
                      count={repositoryCounts?.[provider.id]}
                      onOpen={showView ? () => setActiveRepoConn(provider) : undefined}
                    />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{provider.lastSync}</TableCell>
                  <TableCell>
                    <ConnectionStatus provider={provider} />
                  </TableCell>
                  <TableCell className="text-right">
                    {hasMenu ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={<Button variant="ghost" size="icon-sm" aria-label={`Actions for ${provider.account}`} />}
                        >
                          <MoreHorizontal className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {showManageActions ? (
                            <DropdownMenuItem
                              onClick={() => void handleSync(provider)}
                              disabled={isSyncing}
                              className="gap-2"
                            >
                              <RefreshCw className={`size-3.5 ${isSyncing ? 'animate-spin' : ''}`} />
                              {isSyncing ? 'Syncing…' : 'Sync repositories'}
                            </DropdownMenuItem>
                          ) : null}
                          {showView ? (
                            <DropdownMenuItem onClick={() => setActiveRepoConn(provider)} className="gap-2">
                              <FolderGit2 className="size-3.5" />
                              View repositories
                            </DropdownMenuItem>
                          ) : null}
                          {manageUrl ? (
                            <DropdownMenuItem
                              render={
                                <a href={manageUrl} target="_blank" rel="noopener noreferrer" />
                              }
                              className="gap-2"
                            >
                              <ExternalLink className="size-3.5" />
                              Manage on GitHub
                              <span className="sr-only">opens in a new tab</span>
                            </DropdownMenuItem>
                          ) : null}
                          {showManageActions ? (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                onClick={() => setDeleteTarget(provider)}
                                variant="destructive"
                                className="gap-2"
                              >
                                <Trash2 className="size-3.5" />
                                Disconnect
                              </DropdownMenuItem>
                            </>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>

      <GitRepositoriesDialog
        connection={activeRepoConn}
        open={!!activeRepoConn}
        onOpenChange={(open) => !open && setActiveRepoConn(null)}
        canSync={canManage}
        onSyncComplete={onConnectionChange}
        onRepositoryCount={onRepositoryCount}
        onSuspended={onSuspended}
      />

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && !isDeleting && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{deleteCopy?.title}</AlertDialogTitle>
            <AlertDialogDescription>{deleteCopy?.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault()
                void handleDelete()
              }}
              disabled={isDeleting}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {isDeleting ? 'Disconnecting…' : 'Disconnect'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function accessCell(provider: GitProviderConnection) {
  if (isGitHubAppConnection(provider)) {
    const accountType = accountTypeLabel(provider.accountType)
    const selection = repositorySelectionLabel(provider.repositorySelection)
    if (!accountType && !selection) {
      return <span className="text-sm text-muted-foreground">—</span>
    }
    return (
      <div className="flex flex-col gap-1 text-sm text-foreground">
        {accountType ? <span>{accountType}</span> : null}
        {selection ? <span className="text-muted-foreground">{selection}</span> : null}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      {provider.organizations.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {provider.organizations.map((org) => (
            <Badge key={org} variant="outline" className="text-[10px]">
              {org}
            </Badge>
          ))}
        </div>
      ) : (
        <span className="text-sm text-muted-foreground">—</span>
      )}
      {provider.permissions.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {provider.permissions.map((permission) => (
            <Badge key={permission} variant="outline" className="text-[10px]">
              {permission}
            </Badge>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function RepositoryCount({
  provider,
  count,
  onOpen,
}: {
  provider: GitProviderConnection
  count: RepositoryCountValue | undefined
  onOpen?: () => void
}) {
  if (isGitHubAppConnection(provider)) {
    if (count == null || count === 'loading') {
      return <span className="text-sm text-muted-foreground">Loading count</span>
    }
    if (count === 'unavailable') {
      return <span className="text-sm text-muted-foreground">Unavailable</span>
    }
    return countButton(`${count} repos`, `${count} repositories for ${provider.account}`, onOpen)
  }
  return countButton(
    `${provider.repositoryCount} repos`,
    `${provider.repositoryCount} repositories for ${provider.account}`,
    onOpen,
  )
}

function countButton(label: string, ariaLabel: string, onOpen?: () => void) {
  if (!onOpen) return <span className="text-sm tabular-nums text-foreground">{label}</span>
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-auto p-0 font-mono text-xs hover:underline"
      aria-label={ariaLabel}
      onClick={onOpen}
    >
      {label}
    </Button>
  )
}

function ConnectionStatus({ provider }: { provider: GitProviderConnection }) {
  const view = connectionStatusView(provider.providerStatus, provider.status)
  const tone = TONE_CLASSES[view.tone]
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium',
        tone.bg,
        tone.text,
        tone.border,
      )}
    >
      <span className={cn('size-1.5 shrink-0 rounded-full', tone.dot)} aria-hidden="true" />
      {view.label}
    </span>
  )
}
