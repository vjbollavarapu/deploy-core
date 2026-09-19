import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/platform/status-badge'
import type { PlatformJob } from '@/lib/types'

interface PlatformJobsTableProps {
  jobs: PlatformJob[]
}

export function PlatformJobsTable({ jobs }: PlatformJobsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Job</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Started</TableHead>
          <TableHead>Duration</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {jobs.map((job) => (
          <TableRow key={job.id}>
            <TableCell className="font-mono text-sm text-foreground">{job.name}</TableCell>
            <TableCell>
              <Badge variant="outline" className="text-[10px]">
                {job.type}
              </Badge>
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">{job.startedAt}</TableCell>
            <TableCell className="text-sm text-muted-foreground">{job.duration}</TableCell>
            <TableCell>
              <StatusBadge status={job.status} showDot />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
