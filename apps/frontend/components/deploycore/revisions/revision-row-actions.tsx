'use client'

import Link from 'next/link'
import { Archive, Eye, GitCompareArrows, MoreHorizontal, RotateCcw, Undo2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import { canRollbackTo, formatRevisionNumber, getActiveRevision } from '@/lib/revisions'
import type { Revision } from '@/lib/types'
import { RollbackConfirmDialog } from './rollback-confirm-dialog'

interface RevisionRowActionsProps {
  revision: Revision
  siblings: Revision[]
  onRevisionUpdated?: () => void
}

export function RevisionRowActions({
  revision,
  siblings,
  onRevisionUpdated,
}: RevisionRowActionsProps) {
  const active = useMemo(() => getActiveRevision(siblings), [siblings])
  const [rollbackOpen, setRollbackOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)

  const rollbackEnabled = canRollbackTo(active, revision)
  const compareHref = active
    ? `/revisions/compare?left=${active.id}&right=${revision.id}`
    : `/revisions/compare?left=${revision.id}&right=${revision.id}`

  const handleRedeploy = async () => {
    try {
      await apiClient.post(`/applications/${revision.applicationId}/deployments`, {
        trigger: 'manual',
      })
      toast.success(`Redeploy queued for revision ${formatRevisionNumber(revision.number)}`)
      onRevisionUpdated?.()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Redeploy queued'
      toast.success(msg)
      onRevisionUpdated?.()
    }
  }

  const handleRollback = async () => {
    try {
      await apiClient.post(`/applications/${revision.applicationId}/rollback`, {
        targetRevisionId: revision.id,
      })
      toast.success(`Rollback to revision ${formatRevisionNumber(revision.number)} started`)
      onRevisionUpdated?.()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Rollback initiated'
      toast.success(msg)
      onRevisionUpdated?.()
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for revision ${revision.number}`} />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          <DropdownMenuItem render={<Link href={`/revisions/${revision.id}`} />}>
            <Eye />
            View
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => void handleRedeploy()}>
            <RotateCcw />
            Redeploy
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={!rollbackEnabled || !active}
            onClick={() => setRollbackOpen(true)}
          >
            <Undo2 />
            Rollback
          </DropdownMenuItem>
          <DropdownMenuItem render={<Link href={compareHref} />}>
            <GitCompareArrows />
            Compare
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={revision.archived || revision.traffic > 0}
            onClick={() => setArchiveOpen(true)}
          >
            <Archive />
            Archive
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {active && rollbackEnabled ? (
        <RollbackConfirmDialog
          open={rollbackOpen}
          onOpenChange={setRollbackOpen}
          current={active}
          target={revision}
          onConfirm={handleRollback}
        />
      ) : null}

      <ConfirmDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        title={`Archive revision ${formatRevisionNumber(revision.number)}?`}
        description="Archived revisions remain visible for audit but cannot receive traffic or rollbacks until restored."
        confirmLabel="Archive"
        onConfirm={() => {
          toast.success(`Revision ${formatRevisionNumber(revision.number)} archived`)
          onRevisionUpdated?.()
        }}
      />
    </>
  )
}
