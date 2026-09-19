import { DatabaseBackup } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { StatusBadge } from '@/components/platform/status-badge'
import { getDashboardPanels } from '@/lib/dashboard'
import type { Status } from '@/lib/types'

function backupToneToStatus(status: 'success' | 'warning' | 'critical'): Status {
  if (status === 'success') return 'healthy'
  if (status === 'warning') return 'degraded'
  return 'failed'
}

export function BackupStatusCard() {
  const { backups } = getDashboardPanels()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle className="flex items-center gap-2">
          <DatabaseBackup className="size-3.5 text-muted-foreground" aria-hidden />
          Backup Status
        </CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <ul className="divide-y divide-border">
          {backups.map((backup) => (
            <li
              key={backup.name}
              className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm"
            >
              <div className="min-w-0">
                <p className="truncate font-medium">{backup.name}</p>
                <p className="text-xs text-muted-foreground">Last: {backup.lastBackup}</p>
              </div>
              <StatusBadge status={backupToneToStatus(backup.status)} />
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
