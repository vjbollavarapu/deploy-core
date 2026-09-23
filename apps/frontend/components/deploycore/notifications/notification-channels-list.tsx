'use client'

import { useState } from 'react'
import { Bell, MoreHorizontal, Send, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
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
import { StatusBadge } from '@/components/platform/status-badge'
import { EmptyState } from '@/components/platform/empty-state'
import {
  deleteNotificationChannel,
  emitNotificationTest,
  updateNotificationChannel,
} from '@/lib/integrations'
import type { NotificationChannel } from '@/lib/types'

interface NotificationChannelsListProps {
  channels: NotificationChannel[]
  organizationId?: string
  onChannelChange?: () => void
  onAddChannel?: () => void
}

export function NotificationChannelsList({
  channels,
  organizationId,
  onChannelChange,
  onAddChannel,
}: NotificationChannelsListProps) {
  const [deleteTarget, setDeleteTarget] = useState<NotificationChannel | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [testingId, setTestingId] = useState<string | null>(null)

  const handleToggle = async (channel: NotificationChannel) => {
    try {
      const nextEnabled = channel.status === 'stopped'
      await updateNotificationChannel(channel.id, { enabled: nextEnabled })
      toast.success(
        nextEnabled ? `Channel "${channel.name}" enabled` : `Channel "${channel.name}" disabled`,
      )
      onChannelChange?.()
    } catch (err) {
      toast.error('Failed to toggle channel status', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    }
  }

  const handleTest = async (channel: NotificationChannel) => {
    if (!organizationId) {
      toast.info(`Simulated test notification to ${channel.name} (${channel.target})`)
      return
    }
    try {
      setTestingId(channel.id)
      await emitNotificationTest(organizationId, 'DEPLOYMENT_FAILED', {
        source: 'test-event',
        channel: channel.name,
        timestamp: new Date().toISOString(),
        message: `DeployCore test alert sent to ${channel.name}`,
      })
      toast.success(`Test alert dispatched to ${channel.name}`)
    } catch (err) {
      toast.error('Failed to send test alert', {
        description: err instanceof Error ? err.message : 'Please check channel endpoint.',
      })
    } finally {
      setTestingId(null)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setIsDeleting(true)
      await deleteNotificationChannel(deleteTarget.id)
      toast.success(`Deleted channel "${deleteTarget.name}"`)
      setDeleteTarget(null)
      onChannelChange?.()
    } catch (err) {
      toast.error('Failed to delete channel', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    } finally {
      setIsDeleting(false)
    }
  }

  if (channels.length === 0) {
    return (
      <div className="p-6">
        <EmptyState
          icon={Bell}
          title="No notification channels"
          description="Add Email, Slack, Teams, Discord, Telegram, or Webhook channels to receive operational alerts."
          action={
            onAddChannel ? (
              <Button size="sm" onClick={onAddChannel}>
                Add Channel
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
        {channels.map((channel) => {
          const isTesting = testingId === channel.id
          const isEnabled = channel.status !== 'stopped'
          return (
            <div key={channel.id} className="flex items-center justify-between gap-4 px-4 py-3">
              <div className="flex min-w-0 items-center gap-3">
                <Badge variant="secondary" className="shrink-0 text-[10px]">
                  {channel.type}
                </Badge>
                <div className="flex min-w-0 flex-col gap-0.5">
                  <span className="text-sm font-medium text-foreground">{channel.name}</span>
                  <span className="truncate font-mono text-xs text-muted-foreground">
                    {channel.target}
                  </span>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <StatusBadge status={channel.status} showDot />
                <Switch
                  checked={isEnabled}
                  onCheckedChange={() => void handleToggle(channel)}
                  aria-label={`Toggle ${channel.name}`}
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
                      onClick={() => void handleTest(channel)}
                      disabled={isTesting || !isEnabled}
                      className="gap-2"
                    >
                      <Send className={`size-3.5 ${isTesting ? 'animate-spin' : ''}`} />
                      Send Test Alert
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onClick={() => setDeleteTarget(channel)}
                      className="gap-2 text-destructive focus:text-destructive"
                    >
                      <Trash2 className="size-3.5" />
                      Delete Channel
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          )
        })}
      </div>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Notification Channel?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove channel{' '}
              <span className="font-semibold text-foreground">{deleteTarget?.name}</span> ({deleteTarget?.target}).
              Existing policies referencing this channel will no longer be able to deliver alerts to it.
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
              {isDeleting ? 'Deleting…' : 'Delete Channel'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
