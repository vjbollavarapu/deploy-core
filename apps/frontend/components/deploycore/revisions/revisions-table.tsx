'use client'

import Link from 'next/link'
import { Progress } from '@/components/ui/progress'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { StatusBadge } from '@/components/platform/status-badge'
import type { Revision } from '@/lib/types'
import { RevisionRowActions } from './revision-row-actions'

interface RevisionsTableProps {
  revisions: Revision[]
  showApplication?: boolean
  onRevisionUpdated?: () => void
}

export function RevisionsTable({
  revisions,
  showApplication = true,
  onRevisionUpdated,
}: RevisionsTableProps) {
  const byApp = (applicationId: string) =>
    revisions.filter((r) => r.applicationId === applicationId)

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Revision</TableHead>
          {showApplication && <TableHead>Application</TableHead>}
          <TableHead>Status</TableHead>
          <TableHead>Traffic</TableHead>
          <TableHead>Commit</TableHead>
          <TableHead>Image digest</TableHead>
          <TableHead>Creator</TableHead>
          <TableHead>Created</TableHead>
          <TableHead>Runtime</TableHead>
          <TableHead className="w-10 text-right">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {revisions.map((rev) => (
          <TableRow key={rev.id} className={rev.archived ? 'opacity-60' : undefined}>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <Link
                  href={`/revisions/${rev.id}`}
                  className="font-mono text-sm font-medium text-foreground hover:underline"
                >
                  {rev.number}
                </Link>
                {rev.archived ? (
                  <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                    Archived
                  </span>
                ) : null}
              </div>
            </TableCell>
            {showApplication && (
              <TableCell>
                <Link
                  href={`/applications/${rev.applicationId}`}
                  className="text-foreground hover:underline"
                >
                  {rev.application}
                </Link>
              </TableCell>
            )}
            <TableCell>
              <StatusBadge status={rev.status} showDot />
            </TableCell>
            <TableCell>
              <div className="flex items-center gap-2">
                <Progress value={rev.traffic} className="w-20 flex-none gap-0" />
                <span className="font-mono text-xs text-muted-foreground">{rev.traffic}%</span>
              </div>
            </TableCell>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <span className="font-mono text-xs text-foreground">{rev.commit}</span>
                <span className="max-w-48 truncate text-xs text-muted-foreground">
                  {rev.commitMessage}
                </span>
              </div>
            </TableCell>
            <TableCell>
              <span className="font-mono text-[11px] text-muted-foreground" title={rev.imageDigest}>
                {rev.imageDigest.slice(0, 19)}…
              </span>
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">{rev.createdBy.name}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{rev.createdAt}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{rev.runtime}</TableCell>
            <TableCell className="text-right">
              <RevisionRowActions
                revision={rev}
                siblings={byApp(rev.applicationId)}
                onRevisionUpdated={onRevisionUpdated}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
