'use client'

import { useState } from 'react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import type { NotificationPolicy } from '@/lib/types'

interface NotificationPoliciesListProps {
  policies: NotificationPolicy[]
}

export function NotificationPoliciesList({ policies }: NotificationPoliciesListProps) {
  const [enabled, setEnabled] = useState<Record<string, boolean>>(
    Object.fromEntries(policies.map((policy) => [policy.id, policy.enabled])),
  )

  return (
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
          <Switch
            checked={enabled[policy.id]}
            onCheckedChange={(value) => {
              setEnabled((prev) => ({ ...prev, [policy.id]: value }))
              toast.success(value ? `${policy.name} enabled` : `${policy.name} disabled`)
            }}
            aria-label={`Toggle ${policy.name}`}
          />
        </div>
      ))}
    </div>
  )
}
