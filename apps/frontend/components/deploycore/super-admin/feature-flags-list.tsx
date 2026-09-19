'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Progress } from '@/components/ui/progress'
import type { FeatureFlag } from '@/lib/types'

interface FeatureFlagsListProps {
  flags: FeatureFlag[]
}

export function FeatureFlagsList({ flags }: FeatureFlagsListProps) {
  const [enabled, setEnabled] = useState<Record<string, boolean>>(
    Object.fromEntries(flags.map((f) => [f.id, f.enabled])),
  )

  return (
    <div className="flex flex-col divide-y divide-border">
      {flags.map((flag) => (
        <div key={flag.id} className="flex items-center justify-between gap-4 px-4 py-3.5">
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-foreground">{flag.name}</span>
              <Badge variant="outline" className="text-[10px]">
                {flag.environment}
              </Badge>
            </div>
            <span className="font-mono text-xs text-muted-foreground">{flag.key}</span>
            {enabled[flag.id] && (
              <div className="flex items-center gap-2 pt-1">
                <Progress value={flag.rollout} className="w-32 flex-none gap-0" />
                <span className="font-mono text-xs text-muted-foreground">{flag.rollout}% rollout</span>
              </div>
            )}
          </div>
          <Switch
            checked={enabled[flag.id]}
            onCheckedChange={(v) => setEnabled((prev) => ({ ...prev, [flag.id]: v }))}
          />
        </div>
      ))}
    </div>
  )
}
