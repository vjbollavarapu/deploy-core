'use client'

import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { CodeBlock } from '@/components/platform/code-block'
import { SecretField } from '@/components/platform/secret-field'
import { StatusBadge } from '@/components/platform/status-badge'
import { cn } from '@/lib/utils'
import type { Webhook } from '@/lib/types'

interface WebhooksTableProps {
  webhooks: Webhook[]
}

export function WebhooksTable({ webhooks }: WebhooksTableProps) {
  const [active, setActive] = useState<Webhook | null>(null)
  const [enabledMap, setEnabledMap] = useState<Record<string, boolean>>(
    Object.fromEntries(webhooks.map((webhook) => [webhook.id, webhook.enabled])),
  )

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
                    {webhook.events.map((event) => (
                      <Badge key={event} variant="outline" className="text-[10px]">
                        {event}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {webhook.signingSecretMasked}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Switch
                    checked={enabledMap[webhook.id]}
                    onCheckedChange={(checked) => {
                      setEnabledMap((prev) => ({ ...prev, [webhook.id]: checked }))
                      toast.success(
                        checked ? `${webhook.name} enabled` : `${webhook.name} disabled`,
                      )
                    }}
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
        <SheetContent className="sm:max-w-lg">
          {active ? (
            <>
              <SheetHeader>
                <SheetTitle>{active.name}</SheetTitle>
                <SheetDescription>
                  Endpoint, signing secret, subscribed events, and delivery history with retries.
                </SheetDescription>
              </SheetHeader>
              <div className="flex flex-col gap-4 overflow-y-auto px-4 pb-6">
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">Endpoint</span>
                  <span className="break-all font-mono text-xs">{active.endpoint}</span>
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">Signing secret</span>
                  <SecretField value={active.signingSecretMasked} neverReveal />
                  <p className="text-[11px] text-muted-foreground">
                    Secrets are never returned in plaintext. Rotate from the API to issue a new value.
                  </p>
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">Events</span>
                  <div className="flex flex-wrap gap-1">
                    {active.events.map((event) => (
                      <Badge key={event} variant="secondary" className="text-[10px]">
                        {event}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                  <span className="text-sm">Enabled</span>
                  <Switch
                    checked={enabledMap[active.id]}
                    aria-label={`Toggle ${active.name}`}
                    onCheckedChange={(checked) => {
                      setEnabledMap((prev) => ({ ...prev, [active.id]: checked }))
                      toast.success(checked ? `${active.name} enabled` : `${active.name} disabled`)
                    }}
                  />
                </div>

                <div className="flex flex-col gap-2">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Deliveries
                  </span>
                  {active.deliveries.map((delivery) => (
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
                      <CodeBlock code={delivery.request} label="Request" />
                      <CodeBlock code={delivery.response} label="Response" />
                    </div>
                  ))}
                </div>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </>
  )
}
