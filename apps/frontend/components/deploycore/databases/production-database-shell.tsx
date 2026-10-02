'use client'

import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { DatabaseSubnav } from '@/components/platform/database-subnav'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient } from '@/lib/api'
import { loadProductionDatabase, type ProductionDatabase } from '@/lib/control-plane/database-read'
import type { Status } from '@/lib/types'

const DatabaseDetailContext = createContext<ProductionDatabase | null>(null)

export function useProductionDatabase(): ProductionDatabase | null {
  return useContext(DatabaseDetailContext)
}

export function ProductionDatabaseShell({
  databaseId,
  children,
}: {
  databaseId: string
  children: ReactNode
}) {
  const [database, setDatabase] = useState<ProductionDatabase | null>(null)
  const [phase, setPhase] = useState<'loading' | 'ready' | 'missing' | 'error'>('loading')
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    void loadProductionDatabase(apiClient, databaseId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setDatabase(result.value)
        setPhase('ready')
        return
      }
      setDatabase(null)
      if (result.kind === 'not-found') {
        setPhase('missing')
        return
      }
      setError(result.message)
      setPhase('error')
    })
    return () => {
      cancelled = true
    }
  }, [databaseId, attempt])

  if (phase === 'loading') {
    return (
      <PageContainer density="wide">
        <LoadingState label="Loading database…" />
      </PageContainer>
    )
  }

  if (phase === 'missing') {
    return (
      <PageContainer density="wide">
        <ErrorState title="Database not found" message={`No database exists for ${databaseId}.`} />
      </PageContainer>
    )
  }

  if (phase === 'error' || !database) {
    return (
      <PageContainer density="wide">
        <ErrorState
          title="Could not load database"
          message={error ?? 'Unable to reach the control plane'}
          onRetry={() => {
            setPhase('loading')
            setError(null)
            setAttempt((value) => value + 1)
          }}
        />
      </PageContainer>
    )
  }

  return (
    <DatabaseDetailContext.Provider value={database}>
      <PageContainer density="wide">
        <ResourceHeader
          title={database.name}
          description={`${database.type} ${database.version}`}
          breadcrumbs={[
            { label: 'Databases', href: '/databases' },
            { label: database.name },
          ]}
          badges={<StatusBadge status={database.status as Status} />}
          meta={
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>{database.environment}</span>
              <span>{database.server}</span>
              <span className="font-mono">{database.id}</span>
            </div>
          }
        />
        <DatabaseSubnav databaseId={databaseId} />
        {children}
      </PageContainer>
    </DatabaseDetailContext.Provider>
  )
}
