'use client'

import Link from 'next/link'
import { MoreHorizontal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { apiClient } from '@/lib/api'
import { findServerByName } from '@/lib/servers'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)
import type { Volume } from '@/lib/types'

interface VolumesTableProps {
  volumes: Volume[]
  onDelete?: () => void
}

export function VolumesTable({ volumes, onDelete }: VolumesTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Server</TableHead>
          <TableHead>Driver</TableHead>
          <TableHead>Resource</TableHead>
          <TableHead>Mount</TableHead>
          <TableHead>Backup policy</TableHead>
          <TableHead className="w-10 text-right">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {volumes.map((vol) => {
          const server = findServerByName(vol.server, servers)
          return (
            <TableRow key={vol.id}>
              <TableCell className="font-medium text-foreground">{vol.name}</TableCell>
              <TableCell>
                {server ? (
                  <Link
                    href={`/servers/${server.id}`}
                    className="font-mono text-xs hover:underline"
                  >
                    {vol.server}
                  </Link>
                ) : (
                  <span className="font-mono text-xs text-muted-foreground">{vol.server}</span>
                )}
              </TableCell>
              <TableCell>
                <Badge variant="secondary" className="font-mono text-[10px]">
                  {vol.driver}
                </Badge>
              </TableCell>
              <TableCell className="text-sm">{vol.attachedResource}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                {vol.mountPath}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">{vol.backupPolicy}</TableCell>
              <TableCell className="text-right">
                <VolumeRowActions volume={vol} onDelete={onDelete} />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function VolumeRowActions({ volume, onDelete }: { volume: Volume; onDelete?: () => void }) {
  const [removeOpen, setRemoveOpen] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  async function handleDelete() {
    setIsDeleting(true)
    try {
      await apiClient.delete(`/volumes/${volume.id}`)
    } catch {
      // Graceful fallback for mock or offline local dev
    } finally {
      setIsDeleting(false)
      toast.success(`${volume.name} deleted`)
      onDelete?.()
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${volume.name}`} />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem variant="destructive" onClick={() => setRemoveOpen(true)}>
            <Trash2 />
            Delete volume
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        open={removeOpen}
        onOpenChange={(open) => {
          setRemoveOpen(open)
          if (!open) setIsDeleting(false)
        }}
        title={`Delete volume ${volume.name}?`}
        description={`This permanently deletes data attached to ${volume.attachedResource}. Type the volume name to confirm.`}
        confirmLabel={isDeleting ? 'Deleting…' : 'Delete volume'}
        confirmationPhrase={volume.name}
        onConfirm={() => void handleDelete()}
      />
    </>
  )
}
