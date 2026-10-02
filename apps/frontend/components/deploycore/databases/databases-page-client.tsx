'use client'

import { useEffect, useState } from 'react'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { CreateDatabaseDialog } from './create-database-dialog'
import { DatabasesFilterTable } from './databases-filter-table'
import { apiClient } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { loadProductionDatabaseList } from '@/lib/control-plane/database-read'
import type { DatabaseInstance } from '@/lib/types'

export function DatabasesPageClient() {
  const { activeOrg } = useOrganization()
  const [databases, setDatabases] = useState<DatabaseInstance[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadProductionDatabaseList(apiClient, activeOrg.id).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setDatabases(result.value)
        setError(null)
        return
      }
      setDatabases(null)
      setError(result.kind === 'error' ? result.message : 'Databases were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, attempt])

  function reload() {
    setDatabases(null)
    setError(null)
    setAttempt((value) => value + 1)
  }

  if (!activeOrg?.id) {
    return (
      <PageContainer density="wide">
        <ErrorState title="Could not load databases" message="Select an organization to load databases." />
      </PageContainer>
    )
  }

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Databases"
        description="Managed PostgreSQL instances. Each database is provisioned on a selected environment and server."
        actions={<CreateDatabaseDialog onCreated={reload} />}
      />
      {error ? (
        <ErrorState title="Could not load databases" message={error} onRetry={reload} />
      ) : !databases ? (
        <LoadingState variant="table" rows={6} label="Loading databases…" />
      ) : (
        <DatabasesFilterTable databases={databases} />
      )}
    </PageContainer>
  )
}
