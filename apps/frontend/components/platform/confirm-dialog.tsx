'use client'

import { useState, type ReactElement, type ReactNode } from 'react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export interface ConfirmDialogProps {
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  confirmationPhrase?: string
  pending?: boolean
  onConfirm: () => void | Promise<void>
}

export function ConfirmDialog({
  trigger,
  open: controlledOpen,
  onOpenChange,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  destructive = false,
  confirmationPhrase,
  pending = false,
  onConfirm,
}: ConfirmDialogProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false)
  const [value, setValue] = useState('')
  const isControlled = controlledOpen !== undefined
  const open = isControlled ? controlledOpen : uncontrolledOpen

  function setOpen(next: boolean) {
    if (!isControlled) setUncontrolledOpen(next)
    onOpenChange?.(next)
    if (!next) setValue('')
  }

  const requiresPhrase = Boolean(confirmationPhrase)
  const canConfirm = (!requiresPhrase || value === confirmationPhrase) && !pending

  async function handleConfirm() {
    await onConfirm()
    setOpen(false)
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      {trigger && <AlertDialogTrigger render={trigger as ReactElement} />}
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {requiresPhrase && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="confirm-phrase" className="text-xs text-muted-foreground">
              Type <span className="font-mono font-semibold text-foreground">{confirmationPhrase}</span>{' '}
              to confirm
            </Label>
            <Input
              id="confirm-phrase"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              autoComplete="off"
              disabled={pending}
            />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{cancelLabel}</AlertDialogCancel>
          <Button
            variant={destructive ? 'destructive' : 'default'}
            disabled={!canConfirm}
            onClick={() => void handleConfirm()}
          >
            {pending ? 'Working…' : confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
