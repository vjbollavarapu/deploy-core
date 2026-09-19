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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { TONE_CLASSES } from '@/lib/status'
import { roleNames } from '@/lib/mock-data'
import type { Invitation } from '@/lib/types'
import { cn } from '@/lib/utils'

interface InvitationsTableProps {
  invitations: Invitation[]
}

const statusTone: Record<Invitation['status'], keyof typeof TONE_CLASSES> = {
  pending: 'info',
  accepted: 'success',
  expired: 'warning',
  revoked: 'critical',
}

export function InvitationsTable({ invitations }: InvitationsTableProps) {
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<string>(roleNames[3])
  const [revokeTarget, setRevokeTarget] = useState<Invitation | null>(null)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex justify-end px-4 pt-4">
        <Button size="sm" onClick={() => setOpen(true)}>
          Invite member
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Email</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Teams</TableHead>
            <TableHead>Invited by</TableHead>
            <TableHead>Invited</TableHead>
            <TableHead>Expires</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-28" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {invitations.map((invitation) => {
            const tone = TONE_CLASSES[statusTone[invitation.status]]
            return (
              <TableRow key={invitation.id}>
                <TableCell className="font-medium text-foreground">{invitation.email}</TableCell>
                <TableCell>
                  <Badge variant="secondary" className="text-[10px]">
                    {invitation.role}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {invitation.teams.map((team) => (
                      <Badge key={team} variant="outline" className="text-[10px]">
                        {team}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{invitation.invitedBy}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{invitation.invitedAt}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{invitation.expiresAt}</TableCell>
                <TableCell>
                  <span className={cn('text-xs font-medium capitalize', tone.text)}>
                    {invitation.status}
                  </span>
                </TableCell>
                <TableCell>
                  {invitation.status === 'pending' ? (
                    <Button variant="ghost" size="sm" onClick={() => setRevokeTarget(invitation)}>
                      Revoke
                    </Button>
                  ) : (
                    <span className="text-xs text-muted-foreground">—</span>
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Invite member</DialogTitle>
            <DialogDescription>
              Send an invitation with one of the default roles: {roleNames.join(', ')}.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-2">
              <Label htmlFor="invite-email">Email</Label>
              <Input
                id="invite-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="teammate@company.com"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="invite-role">Role</Label>
              <Select value={role} onValueChange={(v) => setRole(v ?? roleNames[3])}>
                <SelectTrigger id="invite-role" className="w-full" aria-label="Invitation role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {roleNames.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              disabled={!email.trim()}
              onClick={() => {
                toast.success(`Invitation sent to ${email}`)
                setEmail('')
                setOpen(false)
              }}
            >
              Send invite
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!revokeTarget}
        onOpenChange={(next) => !next && setRevokeTarget(null)}
        title={`Revoke invitation to ${revokeTarget?.email}?`}
        description="The invite link will stop working immediately. You can send a new invitation later."
        confirmLabel="Revoke invitation"
        destructive
        onConfirm={() => {
          toast.success(`Invitation to ${revokeTarget?.email} revoked`)
          setRevokeTarget(null)
        }}
      />
    </div>
  )
}
