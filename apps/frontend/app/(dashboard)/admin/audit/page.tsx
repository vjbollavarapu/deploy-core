'use client'

import { useCallback } from 'react'
import { ShieldCheck } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { EmptyState } from '@/components/platform/empty-state'
import { AuditLogTable } from '@/components/deploycore/audit/audit-log-table'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import { fetchAuditLogs, mapWireAuditRecordToEntry } from '@/lib/rbac'
import { auditLog as rawAuditLog } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { AuditLogEntry } from '@/lib/types'

export default function AdminAuditPage() {
  const { activeOrg } = useOrganization()
  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  const loadAuditLogs = useCallback(async (): Promise<AuditLogEntry[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawAuditLog) : []
    try {
      const res = await fetchAuditLogs(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map(mapWireAuditRecordToEntry)
      }
      return isDemo ? getDemoFixtures(rawAuditLog) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawAuditLog)
      throw err
    }
  }, [orgId, isDemo])

  const { data, isLoading, error, reload } = useApiQuery(loadAuditLogs)
  const auditEntries = data || []

  return (
    <Card>
      <CardContent className="p-0">
        {isLoading ? (
          <div className="py-12">
            <LoadingState label="Loading audit trail…" />
          </div>
        ) : error && !isDemo ? (
          <div className="p-6">
            <ErrorState
              title="Failed to load audit trail"
              message={error}
              onRetry={reload}
            />
          </div>
        ) : auditEntries.length === 0 ? (
          <div className="p-8">
            <EmptyState
              icon={ShieldCheck}
              title="No audit events found"
              description="Platform management and administrative events will appear here."
            />
          </div>
        ) : (
          <AuditLogTable entries={auditEntries} />
        )}
      </CardContent>
    </Card>
  )
}
