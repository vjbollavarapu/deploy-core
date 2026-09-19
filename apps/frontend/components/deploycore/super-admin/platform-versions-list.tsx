import { StatusBadge } from '@/components/platform/status-badge'
import type { Status } from '@/lib/types'

interface PlatformVersionsListProps {
  versions: { component: string; version: string; status: Status; updatedAt: string }[]
}

export function PlatformVersionsList({ versions }: PlatformVersionsListProps) {
  return (
    <div className="flex flex-col divide-y divide-border">
      {versions.map((v) => (
        <div key={v.component} className="flex items-center justify-between gap-4 px-4 py-3">
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-medium text-foreground">{v.component}</span>
            <span className="text-xs text-muted-foreground">Updated {v.updatedAt}</span>
          </div>
          <div className="flex items-center gap-3">
            <span className="font-mono text-xs text-muted-foreground">{v.version}</span>
            <StatusBadge status={v.status} showDot />
          </div>
        </div>
      ))}
    </div>
  )
}
