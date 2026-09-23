'use client'

import { useCallback, useEffect, useState } from 'react'
import { ChevronRight, KeyRound, RefreshCw, Send, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
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
import { CodeBlock } from '@/components/platform/code-block'
import { SecretField } from '@/components/platform/secret-field'
import { StatusBadge } from '@/components/platform/status-badge'
import { LoadingState } from '@/components/platform/loading-state'
import { cn } from '@/lib/utils'
import {
  deleteWebhook,
  emitWebhookTest,
  fetchWebhookDeliveries,
  mapWireWebhookDelivery,
  updateWebhook,
} from '@/lib/integrations'
import type { Webhook, WebhookDelivery } from '@/lib/types'

interface WebhooksTableProps {
  webhooks: Webhook[]
  organizationId?: string
  onWebhookChange?: () => void
}

export function WebhooksTable({ webhooks, organizationId, onWebhookChange }: WebhooksTableProps) {
  const [active, setActive] = useState<Webhook | null>(null)
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([])
  const [loadingDeliveries, setLoadingDeliveries] = useState(false)
  const [rotatingSecret, setRotatingSecret] = useState(false)
  const [rotateConfirm, setRotateConfirm] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [isTesting, setIsTesting] = useState(false)

  const loadDeliveries = useCallback(async (webhookId: string) => {
    try {
      setLoadingDeliveries(true)
      const res = await fetchWebhookDeliveries(webhookId)
      if (res?.items) {
        setDeliveries(res.items.map((d) => mapWireWebhookDelivery(d)))
      } else {
        setDeliveries([])
      }
    } catch {
      setDeliveries(active?.deliveries || [])
    } finally {
      setLoadingDeliveries(false)
    }
  }, [active?.deliveries])

  useEffect(() => {
    let cancelled = false
    if (!active?.id) {
      return
    }

    fetchWebhookDeliveries(active.id)
      .then((res) => {
        if (cancelled) return
        if (res?.items) {
          setDeliveries(res.items.map((d) => mapWireWebhookDelivery(d)))
        } else {
          setDeliveries([])
        }
      })
      .catch(() => {
        if (cancelled) return
        setDeliveries(active.deliveries || [])
      })
      .finally(() => {
        if (!cancelled) setLoadingDeliveries(false)
      })

    return () => {
      cancelled = true
    }
  }, [active?.id, active?.deliveries])

  const handleToggle = async (webhook: Webhook, e?: React.MouseEvent) => {
    e?.stopPropagation()
    try {
      const nextEnabled = !webhook.enabled
      await updateWebhook(webhook.id, { enabled: nextEnabled })
      toast.success(nextEnabled ? `${webhook.name} enabled` : `${webhook.name} disabled`)
      onWebhookChange?.()
      if (active?.id === webhook.id) {
        setActive({ ...active, enabled: nextEnabled })
      }
    } catch (err) {
      toast.error('Failed to toggle webhook state', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    }
  }

  const handleRotateSecret = async () => {
    if (!active) return
    try {
      setRotatingSecret(true)
      const res = await updateWebhook(active.id, { rotateSecret: true })
      setRotateConfirm(false)
      if (res?.secretPlain) {
        toast.success(`Signing secret rotated for ${active.name}`, {
          description: `New secret: ${res.secretPlain} (Copy now; never shown again!)`,
          duration: 12000,
        })
      } else {
        toast.success(`Signing secret rotated for ${active.name}`)
      }
      onWebhookChange?.()
    } catch (err) {
      toast.error('Failed to rotate secret', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    } finally {
      setRotatingSecret(false)
    }
  }

  const handleTestEmit = async () => {
    if (!active || !organizationId) {
      toast.info('Test payload simulated (demo mode)')
      return
    }
    try {
      setIsTesting(true)
      await emitWebhookTest(organizationId, active.events[0] || 'deployment.started', {
        event: active.events[0] || 'deployment.started',
        webhookId: active.id,
        timestamp: new Date().toISOString(),
        test: true,
        message: `DeployCore test event dispatched to ${active.endpoint}`,
      })
      toast.success(`Test webhook dispatched to ${active.endpoint}`)
      await loadDeliveries(active.id)
      onWebhookChange?.()
    } catch (err) {
      toast.error('Failed to emit test event', {
        description: err instanceof Error ? err.message : 'Please check endpoint connectivity.',
      })
    } finally {
      setIsTesting(false)
    }
  }

  const handleDelete = async () => {
    if (!active) return
    try {
      setIsDeleting(true)
      await deleteWebhook(active.id)
      toast.success(`Deleted webhook "${active.name}"`)
      setDeleteConfirm(false)
      setActive(null)
      onWebhookChange?.()
    } catch (err) {
      toast.error('Failed to delete webhook', {
        description: err instanceof Error ? err.message : 'Please try again.',
      })
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Endpoint</TableHead>
            <TableHead>Events</TableHead>
            <TableHead>Secret</TableHead>
            <TableHead>Enabled</TableHead>
            <TableHead>Deliveries</TableHead>
            <TableHead>Retries</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="w-8" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {webhooks.map((webhook) => {
            const retryCount = webhook.deliveries.filter((d) => d.retried).length
            return (
              <TableRow
                key={webhook.id}
                className="cursor-pointer"
                onClick={() => setActive(webhook)}
              >
                <TableCell className="font-medium text-foreground">{webhook.name}</TableCell>
                <TableCell className="max-w-[14rem] truncate font-mono text-xs text-foreground">
                  {webhook.endpoint}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {webhook.events.slice(0, 3).map((event) => (
                      <Badge key={event} variant="outline" className="text-[10px]">
                        {event}
                      </Badge>
                    ))}
                    {webhook.events.length > 3 && (
                      <Badge variant="secondary" className="text-[10px]">
                        +{webhook.events.length - 3}
                      </Badge>
                    )}
                  </div>
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {webhook.signingSecretMasked}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Switch
                    checked={webhook.enabled}
                    onCheckedChange={() => void handleToggle(webhook)}
                    aria-label={`Toggle ${webhook.name}`}
                  />
                </TableCell>
                <TableCell className="tabular text-muted-foreground">
                  {webhook.deliveries.length}
                  <span className="ml-1 text-xs">· {webhook.latestDelivery}</span>
                </TableCell>
                <TableCell className="tabular text-muted-foreground">{retryCount}</TableCell>
                <TableCell>
                  <StatusBadge status={webhook.status} showDot />
                </TableCell>
                <TableCell>
                  <ChevronRight data-icon className="size-4 text-muted-foreground" />
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      <Sheet open={!!active} onOpenChange={(open) => !open && setActive(null)}>
        <SheetContent className="sm:max-w-lg flex flex-col">
          {active ? (
            <>
              <SheetHeader>
                <SheetTitle>{active.name}</SheetTitle>
                <SheetDescription>
                  Endpoint, signing secret, subscribed events, and delivery history with retries.
                </SheetDescription>
              </SheetHeader>
              <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-4 pb-6">
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">Endpoint</span>
                  <span className="break-all font-mono text-xs text-foreground">{active.endpoint}</span>
                </div>

                <div className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">Signing secret</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setRotateConfirm(true)}
                      className="h-6 gap-1 px-1.5 text-xs text-muted-foreground hover:text-foreground"
                    >
                      <KeyRound className="size-3" />
                      Rotate Secret
                    </Button>
                  </div>
                  <SecretField value={active.signingSecretMasked} neverReveal />
                  <p className="text-[11px] text-muted-foreground">
                    Secrets are sealed at rest. Rotated secrets invalidate previous signatures immediately.
                  </p>
                </div>

                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">Subscribed Events</span>
                  <div className="flex flex-wrap gap-1">
                    {active.events.map((event) => (
                      <Badge key={event} variant="secondary" className="text-[10px] font-mono">
                        {event}
                      </Badge>
                    ))}
                  </div>
                </div>

                <div className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                  <span className="text-sm">Enabled</span>
                  <Switch
                    checked={active.enabled}
                    aria-label={`Toggle ${active.name}`}
                    onCheckedChange={() => void handleToggle(active)}
                  />
                </div>

                <div className="flex items-center justify-between gap-2 pt-1">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleTestEmit}
                    disabled={isTesting}
                    className="gap-1.5 flex-1"
                  >
                    <Send className={`size-3.5 ${isTesting ? 'animate-spin' : ''}`} />
                    {isTesting ? 'Sending…' : 'Send Test Event'}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setDeleteConfirm(true)}
                    className="gap-1.5 text-destructive hover:bg-destructive/10 hover:text-destructive"
                  >
                    <Trash2 className="size-3.5" />
                    Delete
                  </Button>
                </div>

                <div className="flex flex-col gap-2 pt-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      Deliveries & Retries
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => void loadDeliveries(active.id)}
                      disabled={loadingDeliveries}
                      className="h-6 px-1 text-xs"
                    >
                      <RefreshCw className={`size-3 ${loadingDeliveries ? 'animate-spin' : ''}`} />
                    </Button>
                  </div>

                  {loadingDeliveries ? (
                    <div className="py-6">
                      <LoadingState label="Fetching deliveries…" />
                    </div>
                  ) : deliveries.length === 0 ? (
                    <div className="rounded-lg border border-dashed border-border p-4 text-center text-xs text-muted-foreground">
                      No deliveries recorded yet. Click &ldquo;Send Test Event&rdquo; to test endpoint dispatch.
                    </div>
                  ) : (
                    deliveries.map((delivery) => (
                      <div
                        key={delivery.id}
                        className="flex flex-col gap-2 rounded-lg border border-border p-3"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <span className="text-xs text-muted-foreground">{delivery.timestamp}</span>
                          <div className="flex items-center gap-2">
                            {delivery.retried ? (
                              <Badge variant="outline" className="text-[10px]">
                                Retried · {delivery.attempts} attempts
                              </Badge>
                            ) : (
                              <Badge variant="outline" className="text-[10px]">
                                {delivery.attempts} attempt
                              </Badge>
                            )}
                            <Badge
                              variant={delivery.statusCode < 300 ? 'secondary' : 'outline'}
                              className={cn(
                                'font-mono text-[10px]',
                                delivery.statusCode >= 400 && 'text-critical',
                              )}
                            >
                              {delivery.statusCode}
                            </Badge>
                            <span className="text-xs text-muted-foreground">{delivery.latencyMs}ms</span>
                          </div>
                        </div>
                        <CodeBlock code={delivery.request} label="Request Payload" />
                        <CodeBlock code={delivery.response} label="Delivery Result" />
                      </div>
                    ))
                  )}
                </div>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>

      <AlertDialog open={rotateConfirm} onOpenChange={setRotateConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Rotate Signing Secret?</AlertDialogTitle>
            <AlertDialogDescription>
              This will generate a new HMAC-SHA256 signing secret for{' '}
              <span className="font-semibold text-foreground">{active?.name}</span>. The previous secret
              will be immediately invalidated. You must update your receiver server with the new secret.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={rotatingSecret}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                void handleRotateSecret()
              }}
              disabled={rotatingSecret}
            >
              {rotatingSecret ? 'Rotating…' : 'Rotate Secret'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteConfirm} onOpenChange={setDeleteConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Webhook?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete webhook{' '}
              <span className="font-semibold text-foreground">{active?.name}</span> ({active?.endpoint}).
              Future events will no longer be dispatched to this endpoint.
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
              {isDeleting ? 'Deleting…' : 'Delete Webhook'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
