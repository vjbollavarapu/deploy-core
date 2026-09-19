import Link from 'next/link'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import type { DatabaseInstance } from '@/lib/types'

interface DatabasesTableProps {
  databases: DatabaseInstance[]
}

export function DatabasesTable({ databases }: DatabasesTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Instance</TableHead>
          <TableHead>Project</TableHead>
          <TableHead>Server</TableHead>
          <TableHead>Storage</TableHead>
          <TableHead>Backups</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {databases.map((db) => (
          <TableRow key={db.id} className="cursor-pointer">
            <TableCell>
              <Link href={`/databases/${db.id}`} className="flex flex-col gap-1 hover:underline">
                <span className="font-medium text-foreground">{db.name}</span>
                <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <Badge variant="secondary" className="font-mono text-[10px]">
                    {db.type}
                  </Badge>
                  {db.version}
                </span>
              </Link>
            </TableCell>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <span className="text-foreground">{db.project}</span>
                <span className="text-xs text-muted-foreground">{db.environment}</span>
              </div>
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">{db.server}</TableCell>
            <TableCell>
              <ResourceUsageBar
                label=""
                value={Math.round((db.storageUsedGb / db.storageTotalGb) * 100)}
                detail={`${db.storageUsedGb} / ${db.storageTotalGb} GB`}
                size="sm"
                className="w-40"
              />
            </TableCell>
            <TableCell>
              <div className="flex flex-col gap-0.5 text-sm">
                <span className="text-foreground">{db.backups} backups</span>
                <span className="text-xs text-muted-foreground">Last: {db.lastBackup}</span>
              </div>
            </TableCell>
            <TableCell>
              <StatusBadge status={db.status} showDot />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
