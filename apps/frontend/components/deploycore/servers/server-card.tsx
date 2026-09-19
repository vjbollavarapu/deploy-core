import Link from 'next/link'
import { Cpu, HardDrive, Server as ServerIcon } from 'lucide-react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { StatusBadge } from '@/components/platform/status-badge'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import type { Server } from '@/lib/types'

interface ServerCardProps {
  server: Server
}

export function ServerCard({ server }: ServerCardProps) {
  return (
    <Link href={`/servers/${server.id}`} className="block">
      <Card className="h-full transition-colors hover:border-primary/40">
        <CardHeader className="flex-row items-start justify-between gap-2 space-y-0">
          <div className="flex items-start gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <ServerIcon className="size-4" />
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="font-medium text-foreground">{server.name}</span>
              <span className="text-xs text-muted-foreground">
                {server.provider} · {server.region}
              </span>
            </div>
          </div>
          <StatusBadge status={server.status} showDot />
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <ResourceUsageBar label="CPU" value={server.cpu} detail={`${server.cpuCores} cores`} size="sm" />
          <ResourceUsageBar label="Memory" value={server.memory} detail={`${server.memoryTotalGb} GB`} size="sm" />
          <ResourceUsageBar label="Disk" value={server.disk} detail={`${server.diskTotalGb} GB`} size="sm" />
          <div className="flex items-center justify-between border-t border-border pt-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <Cpu className="size-3.5" />
              {server.containers} containers
            </span>
            <span className="flex items-center gap-1.5">
              <HardDrive className="size-3.5" />
              {server.uptime}
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
