'use client'

import { useState } from 'react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import { isIntegrationConnected, REGISTRY_TYPE_LABELS } from '@/lib/integrations'
import type { Registry } from '@/lib/types'

interface RegistriesTableProps {
  registries: Registry[]
}

export function RegistriesTable({ registries }: RegistriesTableProps) {
  const [connectTarget, setConnectTarget] = useState<Registry | null>(null)
  const [registryUrl, setRegistryUrl] = useState('')

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
            <TableHead className="w-28" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {registries.map((registry) => {
            const connected = isIntegrationConnected(registry.status)
            return (
              <TableRow key={registry.id}>
                <TableCell className="font-medium text-foreground">{registry.name}</TableCell>
                <TableCell>
                  <Badge variant="secondary" className="text-[10px]">
                    {REGISTRY_TYPE_LABELS[registry.type]}
                  </Badge>
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{registry.url}</TableCell>
                <TableCell className="tabular text-foreground">{registry.imageCount}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{registry.connectedAt}</TableCell>
                <TableCell>
                  <StatusBadge status={registry.status} showDot />
                </TableCell>
                <TableCell>
                  <Button
                    variant={connected ? 'ghost' : 'outline'}
                    size="sm"
                    onClick={() => {
                      if (connected) {
                        toast.message(`Manage ${REGISTRY_TYPE_LABELS[registry.type]}`, {
                          description: registry.url,
                        })
                        return
                      }
                      setRegistryUrl('')
                      setConnectTarget(registry)
                    }}
                  >
                    {connected ? 'Manage' : 'Connect'}
                  </Button>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      <Dialog open={!!connectTarget} onOpenChange={(open) => !open && setConnectTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Connect {connectTarget ? REGISTRY_TYPE_LABELS[connectTarget.type] : 'registry'}
            </DialogTitle>
            <DialogDescription>
              Register credentials for pulling and pushing OCI images. Secrets are write-only and never
              shown again.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="registry-url">Registry URL</Label>
            <Input
              id="registry-url"
              value={registryUrl}
              onChange={(e) => setRegistryUrl(e.target.value)}
              placeholder={
                connectTarget?.type === 'AWS ECR'
                  ? '123456.dkr.ecr.region.amazonaws.com'
                  : connectTarget?.type === 'Azure Container Registry'
                    ? 'myregistry.azurecr.io'
                    : connectTarget?.type === 'GCP Artifact Registry'
                      ? 'REGION-docker.pkg.dev/PROJECT'
                      : 'registry.example.com'
              }
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConnectTarget(null)}>
              Cancel
            </Button>
            <Button
              disabled={!registryUrl.trim()}
              onClick={() => {
                toast.success(`${connectTarget ? REGISTRY_TYPE_LABELS[connectTarget.type] : 'Registry'} connect started`)
                setConnectTarget(null)
              }}
            >
              Continue
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
