'use client'

import { useState } from 'react'
import { Wrench } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { StatusBadge } from '@/components/platform/status-badge'
import { isMaintenanceMode } from '@/lib/servers'
import type { Server } from '@/lib/types'

interface MaintenanceModeCardProps {
  server: Server
}

export function MaintenanceModeCard({ server }: MaintenanceModeCardProps) {
  const [enabled, setEnabled] = useState(isMaintenanceMode(server))
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [pendingEnable, setPendingEnable] = useState(false)

  function requestToggle(next: boolean) {
    if (next === enabled) return
    if (next) {
      setPendingEnable(true)
      setConfirmOpen(true)
      return
    }
    setEnabled(false)
    toast.success(`Maintenance mode disabled on ${server.name}`)
  }

  return (
    <>
      <Card size="sm" className={enabled ? 'border-info/40' : undefined}>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wrench className="size-4 text-muted-foreground" />
            Maintenance mode
          </CardTitle>
          <CardDescription>
            Drain new placements and pause automated deployments while you patch or reboot this
            host.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5">
            <div className="min-w-0">
              <Label htmlFor="maintenance-mode" className="text-sm font-medium">
                Enable maintenance mode
              </Label>
              <p className="text-xs text-muted-foreground">
                Existing containers keep running. New scheduling is blocked.
              </p>
            </div>
            <Switch
              id="maintenance-mode"
              checked={enabled}
              onCheckedChange={requestToggle}
            />
          </div>
          <ul className="space-y-1.5 text-sm text-muted-foreground">
            <li>• Scheduler will not place new applications on this server</li>
            <li>• Auto-redeploys and rollouts targeting this host are paused</li>
            <li>• Agent heartbeats and metrics continue to stream</li>
            <li>• Exit maintenance only after health checks look healthy</li>
          </ul>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">Current status</span>
            <StatusBadge status={enabled ? 'maintenance' : server.status === 'maintenance' ? 'running' : server.status} />
            {enabled ? (
              <Button type="button" size="sm" variant="outline" onClick={() => requestToggle(false)}>
                Exit maintenance
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          setConfirmOpen(open)
          if (!open) setPendingEnable(false)
        }}
        title={`Enable maintenance mode on ${server.name}?`}
        description="New workloads will not be scheduled on this server until maintenance mode is turned off. Running containers are not stopped automatically."
        confirmLabel="Enable maintenance"
        onConfirm={() => {
          if (!pendingEnable) return
          setEnabled(true)
          setPendingEnable(false)
          toast.success(`Maintenance mode enabled on ${server.name}`)
        }}
      />
    </>
  )
}
