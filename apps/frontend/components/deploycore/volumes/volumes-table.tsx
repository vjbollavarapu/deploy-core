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
import { findServerByName } from '@/lib/servers'
import { servers } from '@/lib/mock-data'
import type { Volume } from '@/lib/types'

interface VolumesTableProps {
  volumes: Volume[]
}

export function VolumesTable({ volumes }: VolumesTableProps) {
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
                <VolumeRowActions volume={vol} />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function VolumeRowActions({ volume }: { volume: Volume }) {
  const [removeOpen, setRemoveOpen] = useState(false)

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
        onOpenChange={setRemoveOpen}
        title={`Delete volume ${volume.name}?`}
        description={`This permanently deletes data attached to ${volume.attachedResource}. Type the volume name to confirm.`}
        confirmLabel="Delete volume"
        confirmationPhrase={volume.name}
        onConfirm={() => {
          toast.success(`${volume.name} deleted`)
        }}
      />
    </>
  )
}
