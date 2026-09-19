'use client'

import { useState, type ReactNode } from 'react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { buildRollbackPrompt } from '@/lib/revisions'
import type { Revision } from '@/lib/types'

interface RollbackConfirmDialogProps {
  current: Revision
  target: Revision
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onConfirm?: () => void | Promise<void>
}

export function RollbackConfirmDialog({
  current,
  target,
  trigger,
  open: controlledOpen,
  onOpenChange,
  onConfirm,
}: RollbackConfirmDialogProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const isControlled = controlledOpen !== undefined
  const open = isControlled ? controlledOpen : uncontrolledOpen

  function setOpen(next: boolean) {
    if (!isControlled) setUncontrolledOpen(next)
    onOpenChange?.(next)
  }

  async function handleConfirm() {
    setPending(true)
    try {
      await onConfirm?.()
      setOpen(false)
    } finally {
      setPending(false)
    }
  }

  const prompt = buildRollbackPrompt(current, target)

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      {trigger ? <AlertDialogTrigger render={trigger as React.ReactElement} /> : null}
      <AlertDialogContent className="max-w-lg">
        <AlertDialogHeader>
          <AlertDialogTitle>Confirm rollback</AlertDialogTitle>
          <AlertDialogDescription className="text-foreground">{prompt}</AlertDialogDescription>
        </AlertDialogHeader>
        <ul className="space-y-2 rounded-lg border border-border bg-muted/40 px-3 py-3 text-sm">
          <li className="flex gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-info" />
            <span>
              <span className="font-medium">No rebuild</span>
              <span className="text-muted-foreground">
                {' '}
                — the existing image for revision {target.number} is reused as-is.
              </span>
            </span>
          </li>
          <li className="flex gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-info" />
            <span>
              <span className="font-medium">Configuration snapshot restored</span>
              <span className="text-muted-foreground">
                {' '}
                — command, resources, env var metadata, secret references, volumes, health checks,
                and domains from revision {target.number} are applied.
              </span>
            </span>
          </li>
          <li className="flex gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-warning" />
            <span>
              <span className="font-medium">Traffic switches only after health verification</span>
              <span className="text-muted-foreground">
                {' '}
                — routing stays on the current revision until the rollback target passes health
                checks.
              </span>
            </span>
          </li>
        </ul>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <Button disabled={pending} onClick={() => void handleConfirm()}>
            {pending ? 'Rolling back…' : 'Rollback'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
