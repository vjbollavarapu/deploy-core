'use client'

import Link from 'next/link'
import { Archive, GitCompareArrows, RotateCcw, Undo2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import { canRollbackTo, formatRevisionNumber } from '@/lib/revisions'
import type { Revision } from '@/lib/types'
import { RollbackConfirmDialog } from './rollback-confirm-dialog'

interface RevisionDetailActionsProps {
  revision: Revision
  active: Revision | undefined
  compareWithId?: string
  onRevisionUpdated?: () => void
}

export function RevisionDetailActions({
  revision,
  active,
  compareWithId,
  onRevisionUpdated,
}: RevisionDetailActionsProps) {
  const [rollbackOpen, setRollbackOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)
  const rollbackEnabled = canRollbackTo(active, revision)
  const compareId = compareWithId ?? active?.id

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
    <div className="flex flex-wrap items-center gap-2">
      <Button
        size="sm"
        variant="outline"
        onClick={() => void handleRedeploy()}
      >
        <RotateCcw data-icon="inline-start" />
        Redeploy
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={!rollbackEnabled || !active}
        onClick={() => setRollbackOpen(true)}
      >
        <Undo2 data-icon="inline-start" />
        Rollback
      </Button>
      {compareId ? (
        <Button
          size="sm"
          variant="outline"
          nativeButton={false}
          render={
            <Link href={`/revisions/compare?left=${compareId}&right=${revision.id}`} />
          }
        >
          <GitCompareArrows data-icon="inline-start" />
          Compare
        </Button>
      ) : null}
      <Button
        size="sm"
        variant="outline"
        disabled={revision.archived || revision.traffic > 0}
        onClick={() => setArchiveOpen(true)}
      >
        <Archive data-icon="inline-start" />
        Archive
      </Button>

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
    </div>
  )
}
