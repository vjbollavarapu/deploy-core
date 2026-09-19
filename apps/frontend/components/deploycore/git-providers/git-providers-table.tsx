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
import { isIntegrationConnected } from '@/lib/integrations'
import type { GitProviderConnection } from '@/lib/types'

interface GitProvidersTableProps {
  providers: GitProviderConnection[]
}

export function GitProvidersTable({ providers }: GitProvidersTableProps) {
  const [connectTarget, setConnectTarget] = useState<GitProviderConnection | null>(null)
  const [account, setAccount] = useState('')

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
            <TableHead className="w-28" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {providers.map((provider) => {
            const connected = isIntegrationConnected(provider.status)
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
                <TableCell className="tabular text-foreground">{provider.repositoryCount}</TableCell>
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
                <TableCell>
                  <Button
                    variant={connected ? 'ghost' : 'outline'}
                    size="sm"
                    onClick={() => {
                      if (connected) {
                        toast.message(`Manage ${provider.type}`, {
                          description: `${provider.account} · ${provider.repositoryCount} repositories`,
                        })
                        return
                      }
                      setAccount('')
                      setConnectTarget(provider)
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
            <DialogTitle>Connect {connectTarget?.type}</DialogTitle>
            <DialogDescription>
              Authorize DeployCore to read repositories
              {connectTarget?.type === 'Generic Git'
                ? ' from a remote Git host.'
                : ` on ${connectTarget?.type}.`}
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="git-account">
              {connectTarget?.type === 'Generic Git' ? 'Remote URL / account' : 'Account or org'}
            </Label>
            <Input
              id="git-account"
              value={account}
              onChange={(e) => setAccount(e.target.value)}
              placeholder={
                connectTarget?.type === 'Generic Git'
                  ? 'git@git.example.com:org'
                  : 'organization-or-user'
              }
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConnectTarget(null)}>
              Cancel
            </Button>
            <Button
              disabled={!account.trim()}
              onClick={() => {
                toast.success(`${connectTarget?.type} connection started`)
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
