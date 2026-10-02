'use client'

import { FileQuestion } from 'lucide-react'
import { DatabaseBackupsPanel } from '@/components/deploycore/databases/database-backups-panel'
import { DatabaseConnectionPanel } from '@/components/deploycore/databases/database-connection-panel'
import { useProductionDatabase } from '@/components/deploycore/databases/production-database-shell'
import { EmptyState } from '@/components/platform/empty-state'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { DatabaseInstance } from '@/lib/types'

function Unavailable({ title, description }: { title: string; description: string }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <EmptyState icon={FileQuestion} title="Nothing to show" description={description} className="border-0" />
      </CardContent>
    </Card>
  )
}

export function ProductionDatabaseSection({ section }: { section: string }) {
  const database = useProductionDatabase()
  if (!database) return null
  const instance = database as DatabaseInstance

  if (section === 'connection') return <DatabaseConnectionPanel database={instance} />
  if (section === 'backups') return <DatabaseBackupsPanel database={instance} />
  if (section === 'metrics' || section === 'logs' || section === 'restore' || section === 'settings') {
    return (
      <Unavailable
        title={section[0].toUpperCase() + section.slice(1)}
        description="This section is not loaded from demo data. The control plane does not supply it on this page."
      />
    )
  }
  return <Unavailable title="Unknown section" description="This database section is not available." />
}
