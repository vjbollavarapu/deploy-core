'use client'

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
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import type { DockerNetwork } from '@/lib/types'

interface NetworksTableProps {
  networks: DockerNetwork[]
}

export function NetworksTable({ networks }: NetworksTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Driver</TableHead>
          <TableHead>Scope</TableHead>
          <TableHead>Environment</TableHead>
          <TableHead>Connected services</TableHead>
          <TableHead className="w-10 text-right">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {networks.map((net) => (
          <TableRow key={net.id}>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <span className="font-medium text-foreground">{net.name}</span>
                <span className="text-xs text-muted-foreground">{net.project}</span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant="secondary" className="font-mono text-[10px]">
                {net.driver}
              </Badge>
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">{net.scope}</TableCell>
            <TableCell>
              <EnvironmentBadge environment={net.environment} />
            </TableCell>
            <TableCell>
              <div className="flex flex-wrap gap-1.5">
                {net.connectedServices.length === 0 ? (
                  <span className="text-xs text-muted-foreground">—</span>
                ) : (
                  net.connectedServices.map((svc) => (
                    <Badge key={svc} variant="outline" className="text-[10px]">
                      {svc}
                    </Badge>
                  ))
                )}
              </div>
            </TableCell>
            <TableCell className="text-right">
              <NetworkRowActions network={net} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function NetworkRowActions({ network }: { network: DockerNetwork }) {
  const [removeOpen, setRemoveOpen] = useState(false)
  const hasServices = network.connectedServices.length > 0

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${network.name}`} />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            variant="destructive"
            disabled={hasServices}
            onClick={() => setRemoveOpen(true)}
          >
            <Trash2 />
            Remove network
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={`Remove network ${network.name}?`}
        description="This deletes the Docker network. Connected services must be detached first."
        confirmLabel="Remove network"
        confirmationPhrase={network.name}
        onConfirm={() => {
          toast.success(`${network.name} removed`)
        }}
      />
    </>
  )
}
