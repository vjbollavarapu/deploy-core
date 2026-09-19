'use client'

import Link from 'next/link'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import { findServerByName } from '@/lib/servers'
import { servers } from '@/lib/mock-data'
import type { Container } from '@/lib/types'
import { ContainerRowActions } from './container-row-actions'

interface ContainersTableProps {
  containers: Container[]
}

export function ContainersTable({ containers }: ContainersTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Container</TableHead>
          <TableHead>Application</TableHead>
          <TableHead>Revision</TableHead>
          <TableHead>Server</TableHead>
          <TableHead>Image</TableHead>
          <TableHead>CPU</TableHead>
          <TableHead>Memory</TableHead>
          <TableHead>Restarts</TableHead>
          <TableHead>State</TableHead>
          <TableHead className="w-10 text-right">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {containers.map((ctr) => {
          const server = findServerByName(ctr.server, servers)
          const serverHref = server ? `/servers/${server.id}` : undefined
          return (
            <TableRow key={ctr.id}>
              <TableCell className="font-mono text-sm font-medium text-foreground">
                {ctr.name}
              </TableCell>
              <TableCell>
                <Link
                  href={`/applications/${ctr.applicationId}`}
                  className="text-foreground hover:underline"
                >
                  {ctr.application}
                </Link>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                {ctr.revision}
              </TableCell>
              <TableCell>
                {serverHref ? (
                  <Link href={serverHref} className="font-mono text-xs hover:underline">
                    {ctr.server}
                  </Link>
                ) : (
                  <span className="font-mono text-xs text-muted-foreground">{ctr.server}</span>
                )}
              </TableCell>
              <TableCell className="max-w-40 truncate font-mono text-xs text-muted-foreground">
                {ctr.image}
              </TableCell>
              <TableCell className="tabular text-sm">{ctr.cpu}%</TableCell>
              <TableCell className="tabular text-sm">
                {ctr.memory}%
                <span className="text-muted-foreground"> / {ctr.memoryLimitMb}MB</span>
              </TableCell>
              <TableCell className="tabular text-muted-foreground">{ctr.restarts}</TableCell>
              <TableCell>
                <StatusBadge status={ctr.status} showDot />
              </TableCell>
              <TableCell className="text-right">
                <ContainerRowActions container={ctr} serverHref={serverHref} />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
