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
import { createWebhook, WEBHOOK_EVENT_OPTIONS } from '@/lib/integrations'
import {
  createWebhookSchema,
  type CreateWebhookValues,
} from '@/lib/validations/integration'

interface CreateWebhookDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  organizationId: string
  onSuccess: () => void
}

export function CreateWebhookDialog({
  open,
  onOpenChange,
  organizationId,
  onSuccess,
}: CreateWebhookDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<CreateWebhookValues>({
    resolver: zodResolver(createWebhookSchema),
    defaultValues: {
      name: '',
      url: '',
      events: ['deployment.started', 'deployment.completed', 'deployment.failed'],
      secret: '',
      failureThreshold: 5,
      enabled: true,
    },
  })

  const selectedEvents = watch('events') || []

  const toggleEvent = (event: string) => {
    const next = selectedEvents.includes(event)
      ? selectedEvents.filter((e) => e !== event)
      : [...selectedEvents, event]
    setValue('events', next, { shouldValidate: true })
  }

  const onSubmit = async (values: CreateWebhookValues) => {
    try {
      setSubmitting(true)
      const res = await createWebhook({
        organizationId,
        name: values.name,
        url: values.url,
        events: values.events,
        secret: values.secret || undefined,
        enabled: values.enabled,
        failureThreshold: values.failureThreshold,
      })

      if (res?.secretPlain) {
        toast.success(`Webhook "${values.name}" created`, {
          description: `Signing secret: ${res.secretPlain} (Copy now; never shown again!)`,
          duration: 10000,
        })
      } else {
        toast.success(`Webhook "${values.name}" created successfully`)
      }

      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      toast.error('Failed to create webhook', {
        description: err instanceof Error ? err.message : 'Please check the URL and retry.',
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Create Outgoing Webhook</DialogTitle>
          <DialogDescription>
            Register an HTTP endpoint to receive structured event notifications signed with HMAC-SHA256.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 py-2 overflow-y-auto">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="webhook-name">Webhook Name</Label>
            <Input
              id="webhook-name"
              placeholder="e.g. Datadog / PagerDuty Dispatcher"
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-destructive">{errors.name.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="webhook-url">Endpoint URL</Label>
            <Input
              id="webhook-url"
              type="url"
              placeholder="https://api.example.com/webhooks/deploycore"
              {...register('url')}
            />
            {errors.url && (
              <p className="text-xs text-destructive">{errors.url.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Subscribed Events</Label>
            <div className="grid grid-cols-1 gap-2 rounded-md border border-border p-3 max-h-48 overflow-y-auto">
              {WEBHOOK_EVENT_OPTIONS.map((event) => (
                <label
                  key={event}
                  className="flex items-center gap-2 text-xs text-foreground cursor-pointer"
                >
                  <Checkbox
                    checked={selectedEvents.includes(event)}
                    onCheckedChange={() => toggleEvent(event)}
                  />
                  <span className="font-mono text-xs">{event}</span>
                </label>
              ))}
            </div>
            {errors.events && (
              <p className="text-xs text-destructive">{errors.events.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="webhook-secret">Signing Secret (Optional)</Label>
            <Input
              id="webhook-secret"
              type="password"
              placeholder="Leave blank to auto-generate a secure random secret"
              {...register('secret')}
            />
            <p className="text-[11px] text-muted-foreground">
              Used to sign payload with `X-DeployCore-Signature-256`. Sealed at rest.
            </p>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="webhook-threshold">Failure Threshold</Label>
            <Input
              id="webhook-threshold"
              type="number"
              min={1}
              max={50}
              {...register('failureThreshold', { valueAsNumber: true })}
            />
            <p className="text-[11px] text-muted-foreground">
              Number of consecutive delivery failures before auto-disabling the webhook.
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
              {submitting ? 'Creating…' : 'Register Webhook'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
