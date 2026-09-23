'use client'

import { useState } from 'react'
import { MoreHorizontal, Shield, Trash2 } from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { DEFAULT_ROLES } from '@/lib/rbac'
import type { TeamMember } from '@/lib/types'

interface TeamMembersTableProps {
  members: TeamMember[]
  onRoleChange?: (memberId: string, roleName: string) => Promise<void>
  onRemoveMember?: (memberId: string) => Promise<void>
}

function initials(name: string) {
  return name
    .split(' ')
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

export function TeamMembersTable({
  members,
  onRoleChange,
  onRemoveMember,
}: TeamMembersTableProps) {
  const [editingMember, setEditingMember] = useState<TeamMember | null>(null)
  const [selectedRole, setSelectedRole] = useState<string>('Developer')
  const [removingMember, setRemovingMember] = useState<TeamMember | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleOpenEdit = (member: TeamMember) => {
    setEditingMember(member)
    setSelectedRole(member.role)
  }

  const handleSaveRole = async () => {
    if (!editingMember || !onRoleChange) return
    try {
      setIsSubmitting(true)
      await onRoleChange(editingMember.id, selectedRole)
      setEditingMember(null)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleConfirmRemove = async () => {
    if (!removingMember || !onRemoveMember) return
    try {
      setIsSubmitting(true)
      await onRemoveMember(removingMember.id)
      setRemovingMember(null)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>User</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Teams</TableHead>
            <TableHead>Last active</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {members.map((member) => (
            <TableRow key={member.id}>
              <TableCell>
                <div className="flex items-center gap-2.5">
                  <Avatar className="size-7">
                    <AvatarFallback className="text-[10px]">{initials(member.name)}</AvatarFallback>
                  </Avatar>
                  <div className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-foreground">{member.name}</span>
                    <span className="text-xs text-muted-foreground">{member.email}</span>
                  </div>
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="secondary" className="text-[10px]">
                  {member.role}
                </Badge>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {member.teams.map((team) => (
                    <Badge key={team} variant="outline" className="text-[10px]">
                      {team}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">{member.lastActive}</TableCell>
              <TableCell>
                <StatusBadge status={member.status} showDot />
              </TableCell>
              <TableCell>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8"
                        aria-label={`Actions for ${member.name}`}
                      >
                        <MoreHorizontal className="size-4 text-muted-foreground" />
                      </Button>
                    }
                  />
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => handleOpenEdit(member)}>
                      <Shield className="mr-2 size-4" />
                      Change Role
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      variant="destructive"
                      onClick={() => setRemovingMember(member)}
                    >
                      <Trash2 className="mr-2 size-4" />
                      Remove Member
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {/* Edit Role Dialog */}
      <Dialog open={!!editingMember} onOpenChange={(open) => !open && setEditingMember(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Change Role for {editingMember?.name}</DialogTitle>
            <DialogDescription>
              Assign one of the default organization roles to configure capability grants.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3 py-2">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-role-select">Assigned Role</Label>
              <Select value={selectedRole} onValueChange={(v) => setSelectedRole(v ?? 'Developer')}>
                <SelectTrigger id="edit-role-select">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {DEFAULT_ROLES.map((role) => (
                    <SelectItem key={role.name} value={role.name}>
                      <div className="flex flex-col">
                        <span className="font-medium">{role.name}</span>
                        <span className="text-xs text-muted-foreground">{role.description}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingMember(null)} disabled={isSubmitting}>
              Cancel
            </Button>
            <Button onClick={handleSaveRole} disabled={isSubmitting}>
              {isSubmitting ? 'Updating…' : 'Save changes'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirm Remove Dialog */}
      <ConfirmDialog
        open={!!removingMember}
        onOpenChange={(next) => !next && setRemovingMember(null)}
        title={`Remove ${removingMember?.name} from organization?`}
        description="The member will lose all access to organization projects, workloads, and environments immediately."
        confirmLabel="Remove Member"
        destructive
        onConfirm={handleConfirmRemove}
      />
    </>
  )
}
