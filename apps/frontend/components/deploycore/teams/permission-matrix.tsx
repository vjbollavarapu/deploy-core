import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import type { PermissionLevel } from '@/lib/types'

interface PermissionMatrixProps {
  roles: readonly string[]
  resources: readonly string[]
  matrix: Record<string, Record<string, PermissionLevel>>
}

const levelStyles: Record<PermissionLevel, string> = {
  full: 'bg-success/15 text-success',
  edit: 'bg-info/15 text-info',
  view: 'bg-muted text-muted-foreground',
  none: 'bg-transparent text-muted-foreground/40',
}

export function PermissionMatrix({ roles, resources, matrix }: PermissionMatrixProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Resource</TableHead>
          {roles.map((role) => (
            <TableHead key={role} className="text-center">
              {role}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {resources.map((resource) => (
          <TableRow key={resource}>
            <TableCell className="font-medium text-foreground">{resource}</TableCell>
            {roles.map((role) => {
              const level = matrix[role]?.[resource] ?? 'none'
              return (
                <TableCell key={role} className="text-center">
                  <span
                    className={cn(
                      'inline-flex min-w-16 items-center justify-center rounded-md px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                      levelStyles[level],
                    )}
                  >
                    {level}
                  </span>
                </TableCell>
              )
            })}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
