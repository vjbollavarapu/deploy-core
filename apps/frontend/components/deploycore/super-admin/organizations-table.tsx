import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/platform/status-badge'
import type { Organization } from '@/lib/types'

interface OrganizationsTableProps {
  organizations: Organization[]
}

export function OrganizationsTable({ organizations }: OrganizationsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Organization</TableHead>
          <TableHead>Plan</TableHead>
          <TableHead>Users</TableHead>
          <TableHead>Servers</TableHead>
          <TableHead>Created</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {organizations.map((org) => (
          <TableRow key={org.id}>
            <TableCell className="font-medium text-foreground">{org.name}</TableCell>
            <TableCell>
              <Badge variant="secondary" className="text-[10px]">
                {org.plan}
              </Badge>
            </TableCell>
            <TableCell className="text-sm text-foreground">{org.userCount}</TableCell>
            <TableCell className="text-sm text-foreground">{org.serverCount}</TableCell>
            <TableCell className="text-sm text-muted-foreground">{org.createdAt}</TableCell>
            <TableCell>
              <StatusBadge status={org.status} showDot />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
