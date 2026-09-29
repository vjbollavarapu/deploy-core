'use client'

import { useEffect, useState } from 'react'
import { HardDrive } from 'lucide-react'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { apiClient, type Page, type Server as WireServer } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import type { WireVolume } from '@/lib/volumes'

function accessLabel(labels?: Record<string, unknown>): string {
  const value = labels?.readOnly
  if (value === true || value === 'true') return 'Read-only'
  return 'Writable'
}

export function ApplicationVolumesPanel({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<Array<WireVolume & { serverName: string }>>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!activeOrg?.id) {
        setLoading(false)
        setRows([])
        return
      }
      setLoading(true)
      setError(null)
      try {
        const [volumePage, serverPage] = await Promise.all([
          apiClient.get<Page<WireVolume>>(
            `/volumes?organizationId=${encodeURIComponent(activeOrg.id)}&limit=100`,
          ),
          apiClient
            .get<Page<WireServer>>(`/servers?organizationId=${encodeURIComponent(activeOrg.id)}&limit=100`)
            .catch(() => null),
        ])
        if (cancelled) return
        const serverNames = new Map<string, string>()
        for (const server of serverPage?.items ?? []) {
          if (server.id && server.name) serverNames.set(server.id, server.name)
        }
        const attached = (volumePage.items ?? []).filter(
          (volume) =>
            volume.attachedResourceType === 'application' && volume.attachedResourceId === applicationId,
        )
        setRows(
          attached.map((volume) => ({
            ...volume,
            serverName: (volume.serverId && serverNames.get(volume.serverId)) || volume.serverId || '—',
          })),
        )
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Unable to load volumes')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId])

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Volumes</CardTitle>
        <CardDescription>Persistent storage attached to this application.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {loading ? (
          <div className="p-4">
            <LoadingState label="Loading volumes" />
          </div>
        ) : error ? (
          <div className="p-4">
            <ErrorState title="Volumes unavailable" message={error} />
          </div>
        ) : rows.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={HardDrive}
              title="No volumes"
              description="Persistent storage attached to this application will appear here."
              className="border-0"
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Mount path</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Access</TableHead>
                  <TableHead>Server</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((volume) => (
                  <TableRow key={volume.id}>
                    <TableCell className="font-medium">{volume.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {volume.mountPath || '—'}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{volume.state || '—'}</TableCell>
                    <TableCell>{accessLabel(volume.labels)}</TableCell>
                    <TableCell className="text-muted-foreground">{volume.serverName}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
