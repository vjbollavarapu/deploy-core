import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import type { TeamMember } from '@/lib/types'

interface TeamMembersTableProps {
  members: TeamMember[]
}

function initials(name: string) {
  return name
    .split(' ')
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

export function TeamMembersTable({ members }: TeamMembersTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>User</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Teams</TableHead>
          <TableHead>Last active</TableHead>
          <TableHead>Status</TableHead>
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
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
