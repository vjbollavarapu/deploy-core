'use client'

import { useCallback } from 'react'
import { ShieldCheck } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
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

export default function AuditLogPage() {
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
    <PageContainer density="wide">
      <PageHeader
        title="Audit Log"
        description="Immutable record of organization events and administrative actions. Secret values are sealed with AES-256-GCM and never displayed."
      />
      <Card>
        <CardHeader>
          <CardTitle>Recorded Events</CardTitle>
          <CardDescription>
            Timestamp, actor, action, resource, project, IP, and result. Open any row to inspect before/after metadata.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="py-12">
              <LoadingState label="Loading audit log records…" />
            </div>
          ) : error && !isDemo ? (
            <div className="p-6">
              <ErrorState
                title="Failed to load audit logs"
                message={error}
                onRetry={reload}
              />
            </div>
          ) : auditEntries.length === 0 ? (
            <div className="p-8">
              <EmptyState
                icon={ShieldCheck}
                title="No audit events recorded"
                description="Administrative actions, membership changes, deployment triggers, and security events will automatically appear here."
              />
            </div>
          ) : (
            <AuditLogTable entries={auditEntries} />
          )}
        </CardContent>
      </Card>
    </PageContainer>
  )
}
