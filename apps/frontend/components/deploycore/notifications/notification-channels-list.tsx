'use client'

import { useState } from 'react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/platform/status-badge'
import { isIntegrationConnected } from '@/lib/integrations'
import type { NotificationChannel } from '@/lib/types'

interface NotificationChannelsListProps {
  channels: NotificationChannel[]
}

export function NotificationChannelsList({ channels }: NotificationChannelsListProps) {
  const [busyId, setBusyId] = useState<string | null>(null)

  return (
    <div className="flex flex-col divide-y divide-border">
      {channels.map((channel) => {
        const connected = isIntegrationConnected(channel.status)
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
              <Button
                variant={connected ? 'ghost' : 'outline'}
                size="sm"
                disabled={busyId === channel.id}
                onClick={() => {
                  setBusyId(channel.id)
                  toast.success(
                    connected
                      ? `${channel.type} channel opened`
                      : `${channel.type} connect started`,
                  )
                  setTimeout(() => setBusyId(null), 400)
                }}
              >
                {connected ? 'Manage' : 'Connect'}
              </Button>
            </div>
          </div>
        )
      })}
    </div>
  )
}
