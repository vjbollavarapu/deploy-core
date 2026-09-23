'use client'

import { useState } from 'react'
import { MoreHorizontal, RefreshCw, Trash2, FolderGit2 } from 'lucide-react'
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
import { StatusBadge } from '@/components/platform/status-badge'
import { GitRepositoriesDialog } from './git-repositories-dialog'
import { deleteGitConnection, syncGitConnection } from '@/lib/integrations'
import type { GitProviderConnection } from '@/lib/types'

interface GitProvidersTableProps {
  providers: GitProviderConnection[]
  onConnectionChange?: () => void
}

export function GitProvidersTable({
  providers,
  onConnectionChange,
}: GitProvidersTableProps) {
  const [activeRepoConn, setActiveRepoConn] = useState<GitProviderConnection | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<GitProviderConnection | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [syncingId, setSyncingId] = useState<string | null>(null)

  const handleSync = async (provider: GitProviderConnection) => {
    try {
      setSyncingId(provider.id)
      await syncGitConnection(provider.id)
      toast.success(`Repositories synchronized from ${provider.type} (${provider.account})`)
      onConnectionChange?.()
    } catch (err) {
      toast.error('Failed to sync provider', {
        description: err instanceof Error ? err.message : 'Please check credentials.',
      })
    } finally {
      setSyncingId(null)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setIsDeleting(true)
      await deleteGitConnection(deleteTarget.id)
      toast.success(`Disconnected ${deleteTarget.type} (${deleteTarget.account})`)
      setDeleteTarget(null)
      onConnectionChange?.()
    } catch (err) {
      toast.error('Failed to disconnect Git provider', {
        description: err instanceof Error ? err.message : 'Please try again later.',
      })
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Provider</TableHead>
            <TableHead>Account</TableHead>
            <TableHead>Organizations</TableHead>
            <TableHead>Repositories</TableHead>
            <TableHead>Permissions</TableHead>
            <TableHead>Last sync</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-28 text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {providers.map((provider) => {
            const isSyncing = syncingId === provider.id
            return (
              <TableRow key={provider.id}>
                <TableCell className="font-medium text-foreground">{provider.type}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{provider.account}</TableCell>
                <TableCell>
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
                </TableCell>
                <TableCell className="tabular text-foreground">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-auto p-0 font-mono text-xs hover:underline"
                    onClick={() => setActiveRepoConn(provider)}
                  >
                    {provider.repositoryCount} repos
                  </Button>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {provider.permissions.length > 0 ? (
                      provider.permissions.map((permission) => (
                        <Badge key={permission} variant="outline" className="text-[10px]">
                          {permission}
                        </Badge>
                      ))
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </div>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{provider.lastSync}</TableCell>
                <TableCell>
                  <StatusBadge status={provider.status} showDot />
                </TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button variant="ghost" size="icon-sm" aria-label="Open actions" />
                      }
                    >
                      <MoreHorizontal className="size-4" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={() => handleSync(provider)}
                        disabled={isSyncing}
                        className="gap-2"
                      >
                        <RefreshCw className={`size-3.5 ${isSyncing ? 'animate-spin' : ''}`} />
                        {isSyncing ? 'Syncing…' : 'Sync Repositories'}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => setActiveRepoConn(provider)}
                        className="gap-2"
                      >
                        <FolderGit2 className="size-3.5" />
                        View Repositories
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onClick={() => setDeleteTarget(provider)}
                        className="gap-2 text-destructive focus:text-destructive"
                      >
                        <Trash2 className="size-3.5" />
                        Disconnect
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      <GitRepositoriesDialog
        connection={activeRepoConn}
        open={!!activeRepoConn}
        onOpenChange={(open) => !open && setActiveRepoConn(null)}
        onSyncComplete={onConnectionChange}
      />

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Disconnect {deleteTarget?.type}?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove the connection credentials for{' '}
              <span className="font-semibold text-foreground">{deleteTarget?.account}</span> and stop
              automatic repository sync. Existing applications deployed from this provider will not be deleted,
              but will no longer receive automatic webhook updates.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                void handleDelete()
              }}
              disabled={isDeleting}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {isDeleting ? 'Disconnecting…' : 'Disconnect Provider'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
