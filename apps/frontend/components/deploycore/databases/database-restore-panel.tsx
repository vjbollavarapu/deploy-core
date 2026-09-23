'use client'

import { useState } from 'react'
import { AlertTriangle, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import { getDatabaseBackupRuns } from '@/lib/databases'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import type { DatabaseInstance } from '@/lib/types'

interface DatabaseRestorePanelProps {
  database: DatabaseInstance
}

const RESTORE_CONFIRM = 'RESTORE'

export function DatabaseRestorePanel({ database }: DatabaseRestorePanelProps) {
  const runs = getDatabaseBackupRuns(database).filter((run) => run.status === 'success')
  const [selectedRunId, setSelectedRunId] = useState(runs[0]?.id ?? '')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const selected = runs.find((run) => run.id === selectedRunId)

  async function onConfirmRestore() {
    if (!selected) return
    if (isDemoModeEnabled()) {
      toast.message('Demo mode: restore is not submitted to the Control Plane.')
      return
    }
    setSubmitting(true)
    try {
      await apiClient.post(`/backups/${selected.id}/restore`, {
        targetDatabaseId: database.id,
        confirm: RESTORE_CONFIRM,
      })
      toast.success(`Restore queued for ${database.name}`)
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : 'Restore is unavailable until a real backup ID from the Control Plane is selected.'
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Alert variant="destructive">
        <AlertTriangle />
        <AlertTitle>Destructive restore</AlertTitle>
        <AlertDescription>
          Restoring overwrites the live data on {database.name}. Connected applications may fail or
          serve stale data until the restore completes. Confirmation phrase must be exactly{' '}
          <span className="font-mono">{RESTORE_CONFIRM}</span> (Control Plane contract).
        </AlertDescription>
      </Alert>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Restore from backup</CardTitle>
          <CardDescription>
            Choose a successful backup run. Type {RESTORE_CONFIRM} to confirm — not the database
            name.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {runs.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No successful backups are available to restore. When not in demo mode, backup runs
              must come from the Control Plane API.
            </p>
          ) : (
            <>
              <div className="flex flex-col gap-2">
                <span className="text-xs text-muted-foreground">Backup run</span>
                <Select
                  value={selectedRunId}
                  onValueChange={(v) => setSelectedRunId(v ?? '')}
                >
                  <SelectTrigger className="w-full max-w-xl">
                    <SelectValue placeholder="Select a backup" />
                  </SelectTrigger>
                  <SelectContent>
                    {runs.map((run) => (
                      <SelectItem key={run.id} value={run.id}>
                        {run.completedAt} · {run.sizeGb} GB · {run.checksum.slice(0, 22)}…
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {selected ? (
                <div className="grid gap-2 rounded-md border border-border p-3 text-sm sm:grid-cols-2">
                  <div>
                    <span className="text-xs text-muted-foreground">Started</span>
                    <p className="font-mono text-xs">{selected.startedAt}</p>
                  </div>
                  <div>
                    <span className="text-xs text-muted-foreground">Completed</span>
                    <p className="font-mono text-xs">{selected.completedAt}</p>
                  </div>
                  <div>
                    <span className="text-xs text-muted-foreground">Destination</span>
                    <p className="font-mono text-xs">{selected.destination}</p>
                  </div>
                  <div>
                    <span className="text-xs text-muted-foreground">Checksum</span>
                    <p className="truncate font-mono text-xs">{selected.checksum}</p>
                  </div>
                </div>
              ) : null}

              <div className="flex flex-wrap gap-2">
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={!selected || submitting}
                  onClick={() => setConfirmOpen(true)}
                >
                  <RotateCcw data-icon="inline-start" />
                  Restore database
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <DestructiveConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={`Restore ${database.name}?`}
        description={`This permanently overwrites the current ${database.type} data on ${database.name} with backup ${selected?.id ?? ''}. Type ${RESTORE_CONFIRM} to confirm.`}
        confirmLabel="Restore now"
        confirmationPhrase={RESTORE_CONFIRM}
        onConfirm={() => {
          void onConfirmRestore()
        }}
      />
    </div>
  )
}
