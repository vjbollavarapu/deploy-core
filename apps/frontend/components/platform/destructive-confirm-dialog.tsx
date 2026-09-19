'use client'

import type { ReactNode } from 'react'
import { ConfirmDialog, type ConfirmDialogProps } from './confirm-dialog'

type DestructiveConfirmDialogProps = Omit<ConfirmDialogProps, 'destructive'> & {
  trigger?: ReactNode
}

/** Destructive actions must always go through confirmation. */
export function DestructiveConfirmDialog(props: DestructiveConfirmDialogProps) {
  return (
    <ConfirmDialog
      {...props}
      destructive
      confirmLabel={props.confirmLabel ?? 'Delete'}
    />
  )
}
