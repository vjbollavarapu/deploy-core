'use client'

import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { createNotificationPolicy } from '@/lib/integrations'
import {
  createNotificationPolicySchema,
  type CreateNotificationPolicyValues,
} from '@/lib/validations/integration'
import type { NotificationChannel } from '@/lib/types'

interface AddPolicyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  organizationId: string
  availableChannels: NotificationChannel[]
  onSuccess: () => void
}

const AVAILABLE_EVENTS = [
  { id: 'DEPLOYMENT_FAILED', label: 'Deployment Failed' },
  { id: 'DEPLOYMENT_SUCCEEDED', label: 'Deployment Succeeded' },
  { id: 'SERVER_OFFLINE', label: 'Server Offline' },
  { id: 'SERVER_DEGRADED', label: 'Server Degraded' },
  { id: 'BACKUP_FAILED', label: 'Backup Failed' },
  { id: 'CERTIFICATE_EXPIRING', label: 'Certificate Expiring' },
  { id: 'DISK_LOW', label: 'Disk Low' },
]

export function AddPolicyDialog({
  open,
  onOpenChange,
  organizationId,
  availableChannels,
  onSuccess,
}: AddPolicyDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<CreateNotificationPolicyValues>({
    resolver: zodResolver(createNotificationPolicySchema),
    defaultValues: {
      name: '',
      eventTypes: ['DEPLOYMENT_FAILED', 'SERVER_OFFLINE'],
      channelIds: [],
      enabled: true,
    },
  })

  const selectedEvents = watch('eventTypes') || []
  const selectedChannels = watch('channelIds') || []

  const toggleEvent = (eventId: string) => {
    const next = selectedEvents.includes(eventId)
      ? selectedEvents.filter((e) => e !== eventId)
      : [...selectedEvents, eventId]
    setValue('eventTypes', next, { shouldValidate: true })
  }

  const toggleChannel = (channelId: string) => {
    const next = selectedChannels.includes(channelId)
      ? selectedChannels.filter((c) => c !== channelId)
      : [...selectedChannels, channelId]
    setValue('channelIds', next, { shouldValidate: true })
  }

  const onSubmit = async (values: CreateNotificationPolicyValues) => {
    try {
      setSubmitting(true)
      await createNotificationPolicy({
        organizationId,
        name: values.name,
        eventTypes: values.eventTypes,
        channelIds: values.channelIds,
        enabled: values.enabled,
      })
      toast.success(`Notification policy "${values.name}" created`)
      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      toast.error('Failed to create notification policy', {
        description: err instanceof Error ? err.message : 'Please check values and retry.',
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Create Notification Policy</DialogTitle>
          <DialogDescription>
            Rules defining when alerts trigger and which notification channels receive them.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 py-2 overflow-y-auto">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="policy-name">Policy Name</Label>
            <Input
              id="policy-name"
              placeholder="e.g. Critical Failures or Production Incidents"
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-destructive">{errors.name.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Trigger Events</Label>
            <div className="grid grid-cols-1 gap-2 rounded-md border border-border p-3">
              {AVAILABLE_EVENTS.map((ev) => (
                <label
                  key={ev.id}
                  className="flex items-center gap-2 text-xs text-foreground cursor-pointer"
                >
                  <Checkbox
                    checked={selectedEvents.includes(ev.id)}
                    onCheckedChange={() => toggleEvent(ev.id)}
                  />
                  <span>{ev.label}</span>
                </label>
              ))}
            </div>
            {errors.eventTypes && (
              <p className="text-xs text-destructive">{errors.eventTypes.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Destination Channels</Label>
            {availableChannels.length === 0 ? (
              <p className="text-xs text-muted-foreground italic">
                No notification channels configured yet. Create a channel first.
              </p>
            ) : (
              <div className="grid grid-cols-1 gap-2 rounded-md border border-border p-3 max-h-40 overflow-y-auto">
                {availableChannels.map((ch) => (
                  <label
                    key={ch.id}
                    className="flex items-center gap-2 text-xs text-foreground cursor-pointer"
                  >
                    <Checkbox
                      checked={selectedChannels.includes(ch.id)}
                      onCheckedChange={() => toggleChannel(ch.id)}
                    />
                    <span className="font-medium">{ch.name}</span>
                    <span className="text-muted-foreground text-[10px]">({ch.type})</span>
                  </label>
                ))}
              </div>
            )}
            {errors.channelIds && (
              <p className="text-xs text-destructive">{errors.channelIds.message}</p>
            )}
          </div>

          <DialogFooter className="pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitting || availableChannels.length === 0}>
              {submitting ? 'Creating…' : 'Create Policy'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
