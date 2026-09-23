'use client'

import { useState } from 'react'
import { MoreHorizontal, Trash2, ShieldCheck } from 'lucide-react'
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
import { deleteRegistry } from '@/lib/integrations'
import { REGISTRY_TYPE_LABELS } from '@/lib/integrations'
import type { Registry } from '@/lib/types'

interface RegistriesTableProps {
  registries: Registry[]
  onRegistryChange?: () => void
}

export function RegistriesTable({ registries, onRegistryChange }: RegistriesTableProps) {
  const [deleteTarget, setDeleteTarget] = useState<Registry | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setIsDeleting(true)
      await deleteRegistry(deleteTarget.id)
      toast.success(`Removed registry "${deleteTarget.name}"`)
      setDeleteTarget(null)
      onRegistryChange?.()
    } catch (err) {
      toast.error('Failed to remove registry', {
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
            <TableHead>Registry</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>URL</TableHead>
            <TableHead>Images</TableHead>
            <TableHead>Connected</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-20 text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {registries.map((registry) => (
            <TableRow key={registry.id}>
              <TableCell className="font-medium text-foreground">{registry.name}</TableCell>
              <TableCell>
                <Badge variant="secondary" className="text-[10px]">
                  {REGISTRY_TYPE_LABELS[registry.type] || registry.type}
                </Badge>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">{registry.url}</TableCell>
              <TableCell className="tabular text-foreground">{registry.imageCount}</TableCell>
              <TableCell className="text-sm text-muted-foreground">{registry.connectedAt}</TableCell>
              <TableCell>
                <StatusBadge status={registry.status} showDot />
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
                      onClick={() =>
                        toast.message(`Registry: ${registry.name}`, {
                          description: `${registry.url} · Type: ${REGISTRY_TYPE_LABELS[registry.type] || registry.type}`,
                        })
                      }
                      className="gap-2"
                    >
                      <ShieldCheck className="size-3.5" />
                      View Details
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onClick={() => setDeleteTarget(registry)}
                      className="gap-2 text-destructive focus:text-destructive"
                    >
                      <Trash2 className="size-3.5" />
                      Delete Registry
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Container Registry?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove the credentials and configuration for registry{' '}
              <span className="font-semibold text-foreground">{deleteTarget?.name}</span> ({deleteTarget?.url}).
              Build and deployment agents will no longer be able to authenticate against this registry.
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
              {isDeleting ? 'Deleting…' : 'Delete Registry'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
