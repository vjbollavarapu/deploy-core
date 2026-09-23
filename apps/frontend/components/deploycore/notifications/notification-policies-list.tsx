'use client'

import { useState } from 'react'
import { MoreHorizontal, ShieldAlert, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { EmptyState } from '@/components/platform/empty-state'
import { deleteNotificationPolicy, updateNotificationPolicy } from '@/lib/integrations'
import type { NotificationPolicy } from '@/lib/types'

interface NotificationPoliciesListProps {
  policies: NotificationPolicy[]
  onPolicyChange?: () => void
  onAddPolicy?: () => void
}

export function NotificationPoliciesList({
  policies,
  onPolicyChange,
  onAddPolicy,
}: NotificationPoliciesListProps) {
  const [deleteTarget, setDeleteTarget] = useState<NotificationPolicy | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleToggle = async (policy: NotificationPolicy) => {
    try {
      const nextEnabled = !policy.enabled
      await updateNotificationPolicy(policy.id, { enabled: nextEnabled })
      toast.success(
        nextEnabled ? `Policy "${policy.name}" enabled` : `Policy "${policy.name}" disabled`,
      )
      onPolicyChange?.()
    } catch (err) {
      toast.error('Failed to update policy', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setIsDeleting(true)
      await deleteNotificationPolicy(deleteTarget.id)
      toast.success(`Deleted policy "${deleteTarget.name}"`)
      setDeleteTarget(null)
      onPolicyChange?.()
    } catch (err) {
      toast.error('Failed to delete policy', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    } finally {
      setIsDeleting(false)
    }
  }

  if (policies.length === 0) {
    return (
      <div className="p-6">
        <EmptyState
          icon={ShieldAlert}
          title="No notification policies"
          description="Create routing rules to forward alerts for failed deployments, degraded nodes, or expired certificates to specific channels."
          action={
            onAddPolicy ? (
              <Button size="sm" onClick={onAddPolicy}>
                Create Policy
              </Button>
            ) : undefined
          }
        />
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-col divide-y divide-border">
        {policies.map((policy) => (
          <div key={policy.id} className="flex items-start justify-between gap-4 px-4 py-3.5">
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-foreground">{policy.name}</span>
              <span className="text-xs text-muted-foreground">{policy.when}</span>
              <div className="flex flex-wrap gap-1.5">
                {policy.conditions.map((condition) => (
                  <Badge key={condition} variant="outline" className="text-[10px]">
                    {condition}
                  </Badge>
                ))}
              </div>
              <div className="flex flex-wrap gap-1.5">
                {policy.channels.map((channel) => (
                  <Badge key={channel} variant="secondary" className="text-[10px]">
                    {channel}
                  </Badge>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                checked={policy.enabled}
                onCheckedChange={() => void handleToggle(policy)}
                aria-label={`Toggle ${policy.name}`}
              />
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button variant="ghost" size="icon-sm" aria-label="Open actions" />
                  }
                >
                  <MoreHorizontal className="size-4" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() => setDeleteTarget(policy)}
                    className="gap-2 text-destructive focus:text-destructive"
                  >
                    <Trash2 className="size-3.5" />
                    Delete Policy
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        ))}
      </div>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Notification Policy?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove policy{' '}
              <span className="font-semibold text-foreground">{deleteTarget?.name}</span>. Alerts matching
              these conditions will no longer be forwarded to its configured channels.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                void handleDelete()
              }}
              disabled={isDeleting}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {isDeleting ? 'Deleting…' : 'Delete Policy'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
