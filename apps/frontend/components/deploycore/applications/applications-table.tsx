import Link from 'next/link'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { projects } from '@/lib/mock-data'
import type { Application } from '@/lib/types'

interface ApplicationsTableProps {
  applications: Application[]
  showProject?: boolean
  /** F4 listing columns: name, project, environment, type, server, revision, status, domain, last deployment */
  listing?: boolean
}

function projectHref(projectId: string, projectName: string) {
  const project = projects.find((p) => p.id === projectId || p.name === projectName)
  return project ? `/projects/${project.slug}` : '/projects'
}

export function ApplicationsTable({
  applications,
  showProject = true,
  listing = false,
}: ApplicationsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          {showProject && <TableHead>Project</TableHead>}
          <TableHead>Environment</TableHead>
          {listing && <TableHead>Type</TableHead>}
          <TableHead>Server</TableHead>
          {listing && <TableHead>Revision</TableHead>}
          <TableHead>Status</TableHead>
          {listing && <TableHead>Domain</TableHead>}
          <TableHead className="text-right">Last deployment</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {applications.map((app) => (
          <TableRow key={app.id}>
            <TableCell>
              <Link href={`/applications/${app.id}`} className="flex flex-col gap-0.5 hover:underline">
                <span className="font-medium text-foreground">{app.name}</span>
                {!listing && <span className="text-xs text-muted-foreground">{app.runtime}</span>}
              </Link>
            </TableCell>
            {showProject && (
              <TableCell>
                <Link
                  href={projectHref(app.projectId, app.project)}
                  className="text-muted-foreground hover:underline"
                >
                  {app.project}
                </Link>
              </TableCell>
            )}
            <TableCell>
              <EnvironmentBadge environment={app.environment} />
            </TableCell>
            {listing && <TableCell className="text-muted-foreground">{app.runtime}</TableCell>}
            <TableCell className="font-mono text-xs text-muted-foreground">{app.server}</TableCell>
            {listing && <TableCell className="font-mono text-xs">{app.revision}</TableCell>}
            <TableCell>
              <StatusBadge status={app.status} />
            </TableCell>
            {listing && (
              <TableCell className="max-w-[10rem] truncate font-mono text-xs text-muted-foreground">
                {app.domain || '—'}
              </TableCell>
            )}
            <TableCell className="text-right text-xs text-muted-foreground">
              {app.lastDeployment}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
