import type { ReactNode } from 'react'
import { notFound } from 'next/navigation'
import { DownloadCloud, RotateCcw } from 'lucide-react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { DatabaseSubnav } from '@/components/platform/database-subnav'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { findDatabase } from '@/lib/databases'
import { databases as rawDatabases } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const databases = getDemoFixtures(rawDatabases)

export default async function DatabaseLayout({
  children,
  params,
}: {
  children: ReactNode
  params: Promise<{ databaseId: string }>
}) {
  const { databaseId } = await params
  const database = findDatabase(databaseId, databases)
  if (!database) notFound()

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={database.name}
        description={`${database.type} ${database.version} · ${database.project}`}
        breadcrumbs={[
          { label: 'Databases', href: '/databases' },
          { label: database.name },
        ]}
        badges={<StatusBadge status={database.status} />}
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{database.environment}</span>
            <span>{database.server}</span>
            <span className="font-mono">
              {database.connectionHost}:{database.port}
            </span>
            <span>Last backup {database.lastBackup}</span>
          </div>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<Link href={`/databases/${databaseId}/backups`} />}
            >
              <DownloadCloud data-icon="inline-start" />
              Backups
            </Button>
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<Link href={`/databases/${databaseId}/restore`} />}
            >
              <RotateCcw data-icon="inline-start" />
              Restore
            </Button>
          </div>
        }
      />
      <DatabaseSubnav databaseId={databaseId} />
      {children}
    </PageContainer>
  )
}
