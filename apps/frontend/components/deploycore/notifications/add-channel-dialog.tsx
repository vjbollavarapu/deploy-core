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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { createNotificationChannel } from '@/lib/integrations'
import {
  createNotificationChannelSchema,
  type CreateNotificationChannelValues,
} from '@/lib/validations/integration'

interface AddChannelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  organizationId: string
  onSuccess: () => void
}

const TARGET_PLACEHOLDERS: Record<string, string> = {
  EMAIL: 'alerts@example.com',
  SLACK: 'https://hooks.slack.com/services/...',
  TEAMS: 'https://outlook.office.com/webhook/...',
  DISCORD: 'https://discord.com/api/webhooks/...',
  TELEGRAM: 'chat_id: -100123456789',
  WEBHOOK: 'https://api.example.com/notifications',
  WHATSAPP: '+1234567890',
}

export function AddChannelDialog({
  open,
  onOpenChange,
  organizationId,
  onSuccess,
}: AddChannelDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<CreateNotificationChannelValues>({
    resolver: zodResolver(createNotificationChannelSchema),
    defaultValues: {
      name: '',
      type: 'SLACK',
      target: '',
      credential: '',
      enabled: true,
    },
  })

  const selectedType = watch('type')

  const onSubmit = async (values: CreateNotificationChannelValues) => {
    try {
      setSubmitting(true)
      const config: Record<string, unknown> = {}
      if (values.type === 'EMAIL') {
        config.email = values.target
      } else if (values.type === 'WEBHOOK' || values.type === 'SLACK' || values.type === 'TEAMS' || values.type === 'DISCORD') {
        config.webhookUrl = values.target
      } else {
        config.target = values.target
      }

      await createNotificationChannel({
        organizationId,
        name: values.name,
        type: values.type,
        config,
        credential: values.credential || undefined,
        enabled: values.enabled,
      })
      toast.success(`Notification channel "${values.name}" created`)
      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      toast.error('Failed to create notification channel', {
        description: err instanceof Error ? err.message : 'Please check the values and retry.',
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add Notification Channel</DialogTitle>
          <DialogDescription>
            Configure destination endpoints for alerts on deployments, server health, backups, and TLS expiration.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="channel-name">Channel Name</Label>
            <Input
              id="channel-name"
              placeholder="e.g. SRE Slack Alerts or Security Team"
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-destructive">{errors.name.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="channel-type">Channel Type</Label>
            <Select
              value={selectedType}
              onValueChange={(val) => setValue('type', val as CreateNotificationChannelValues['type'], { shouldValidate: true })}
            >
              <SelectTrigger id="channel-type">
                <SelectValue placeholder="Select channel type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="EMAIL">Email</SelectItem>
                <SelectItem value="SLACK">Slack</SelectItem>
                <SelectItem value="DISCORD">Discord</SelectItem>
                <SelectItem value="TEAMS">Microsoft Teams</SelectItem>
                <SelectItem value="TELEGRAM">Telegram</SelectItem>
                <SelectItem value="WEBHOOK">Generic Webhook</SelectItem>
                <SelectItem value="WHATSAPP">WhatsApp</SelectItem>
              </SelectContent>
            </Select>
            {errors.type && (
              <p className="text-xs text-destructive">{errors.type.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="channel-target">
              {selectedType === 'EMAIL'
                ? 'Destination Email'
                : selectedType === 'TELEGRAM'
                  ? 'Telegram Chat ID'
                  : selectedType === 'WHATSAPP'
                    ? 'WhatsApp Phone Number'
                    : 'Webhook URL'}
            </Label>
            <Input
              id="channel-target"
              placeholder={TARGET_PLACEHOLDERS[selectedType] || 'Destination'}
              {...register('target')}
            />
            {errors.target && (
              <p className="text-xs text-destructive">{errors.target.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="channel-credential">Bearer Token / Secret Key (Optional)</Label>
            <Input
              id="channel-credential"
              type="password"
              placeholder="Optional auth token for HTTP headers"
              {...register('credential')}
            />
            <p className="text-[11px] text-muted-foreground">
              Sealed with AES-256-GCM. Transmitted securely in webhook authorization headers.
            </p>
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
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Creating…' : 'Create Channel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
