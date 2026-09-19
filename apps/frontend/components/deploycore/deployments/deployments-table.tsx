import Link from 'next/link'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { DEPLOYMENT_PHASE_LABELS, DEPLOYMENT_FAILURE_LABELS } from '@/lib/deployments'
import type { Deployment } from '@/lib/types'

interface DeploymentsTableProps {
  deployments: Deployment[]
  showApplication?: boolean
}

export function DeploymentsTable({ deployments, showApplication = true }: DeploymentsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Deployment</TableHead>
          {showApplication && <TableHead>Application</TableHead>}
          <TableHead>Environment</TableHead>
          <TableHead>Commit</TableHead>
          <TableHead>Author</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Phase</TableHead>
          <TableHead>Duration</TableHead>
          <TableHead className="text-right">Started</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {deployments.map((dep) => (
          <TableRow key={dep.id}>
            <TableCell>
              <Link href={`/deployments/${dep.id}`} className="font-mono text-sm text-foreground hover:underline">
                #{dep.number}
              </Link>
            </TableCell>
            {showApplication && (
              <TableCell>
                <Link href={`/applications/${dep.applicationId}`} className="text-foreground hover:underline">
                  {dep.application}
                </Link>
              </TableCell>
            )}
            <TableCell>
              <EnvironmentBadge environment={dep.environment} />
            </TableCell>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <span className="font-mono text-xs text-foreground">{dep.commit}</span>
                <span className="max-w-48 truncate text-xs text-muted-foreground">{dep.commitMessage}</span>
              </div>
            </TableCell>
            <TableCell className="text-muted-foreground">{dep.author.name}</TableCell>
            <TableCell>
              <StatusBadge status={dep.status} />
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {dep.failureReason
                ? DEPLOYMENT_FAILURE_LABELS[dep.failureReason]
                : DEPLOYMENT_PHASE_LABELS[dep.phase]}
            </TableCell>
            <TableCell className="tabular text-muted-foreground">{dep.duration}</TableCell>
            <TableCell className="text-right text-xs text-muted-foreground">{dep.startedAt}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
