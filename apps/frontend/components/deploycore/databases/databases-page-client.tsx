'use client'

import { useCallback } from 'react'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { CreateDatabaseDialog } from './create-database-dialog'
import { DatabasesFilterTable } from './databases-filter-table'
import { useApiQuery } from '@/hooks/use-api-query'
import { apiClient, type Database, type WireProject, type Server, type WireEnvironment } from '@/lib/api'
import type { DatabaseInstance } from '@/lib/types'

export function DatabasesPageClient() {
  const fetchDatabases = useCallback(async () => {
    const [dbRes, projRes, envRes, serverRes] = await Promise.all([
      apiClient.get<{ databases: Database[] }>('/databases'),
      apiClient.get<{ items: WireProject[] }>('/projects'),
      apiClient.get<{ items: WireEnvironment[] }>('/environments'),
      apiClient.get<{ items: Server[] }>('/servers'),
    ])

    const projMap = new Map(projRes.items?.map(p => [p.id, p]) || [])
    const envMap = new Map(envRes.items?.map(e => [e.id, e]) || [])
    const serverMap = new Map(serverRes.items?.map(s => [s.id, s]) || [])

    return dbRes.databases.map(db => {
      const proj = projMap.get(db.projectId || '')
      const env = envMap.get(db.environmentId || '')
      const server = serverMap.get(db.serverId || '')

      return {
        id: db.id || '',
        name: db.name || 'Unnamed',
        type: 'PostgreSQL',
        version: db.engineVersion || '15',
        project: proj?.name || 'Unknown',
        environment: env?.name || 'Unknown',
        server: server?.name || 'Unknown',
        storageUsedGb: 0,
        storageTotalGb: 10,
        backups: 0,
        lastBackup: 'Never',
        status: db.status?.toLowerCase() === 'running' ? 'healthy' : 'pending',
        dbName: db.databaseName || '',
        port: 5432,
        username: db.username || '',
        connectionHost: server?.hostname || '',
        credentialsRevealAllowed: db.hasCredential || false,
      } as DatabaseInstance
    })
  }, [])

  const { data: databases, isLoading, error, reload } = useApiQuery(fetchDatabases)

  if (isLoading) return <LoadingState label="Loading databases..." />
  if (error) return <ErrorState title="Failed to load databases" message={error} onRetry={reload} />

  function handleCreated() {
    reload()
  }

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Databases"
        description="Managed database instances across all projects. PostgreSQL is the default engine with automated storage volume provisioning and scheduled backup policies."
      />
      <DatabasesFilterTable
        databases={databases || []}
        headerAction={<CreateDatabaseDialog onCreated={handleCreated} />}
      />
    </PageContainer>
  )
}
