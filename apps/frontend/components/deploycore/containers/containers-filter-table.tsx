'use client'

import { useMemo, useState } from 'react'
import { Box } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ContainersTable } from './containers-table'
import type { Container } from '@/lib/types'

interface ContainersFilterTableProps {
  containers: Container[]
  onActionSuccess?: () => void
}

const STATE_FILTERS = [
  { value: 'all', label: 'All states' },
  { value: 'healthy', label: 'Healthy' },
  { value: 'running', label: 'Running' },
  { value: 'deploying', label: 'Deploying' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'stopped', label: 'Stopped' },
] as const

export function ContainersFilterTable({ containers, onActionSuccess }: ContainersFilterTableProps) {
  const [query, setQuery] = useState('')
  const [application, setApplication] = useState('all')
  const [server, setServer] = useState('all')
  const [status, setStatus] = useState('all')

  const availableApplications = useMemo(
    () => Array.from(new Set(containers.map((c) => c.application).filter(Boolean))),
    [containers],
  )

  const availableServers = useMemo(
    () => Array.from(new Set(containers.map((c) => c.server).filter(Boolean))),
    [containers],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return containers.filter((ctr) => {
      const matchesQuery =
        q === '' ||
        ctr.name.toLowerCase().includes(q) ||
        ctr.application.toLowerCase().includes(q) ||
        ctr.server.toLowerCase().includes(q) ||
        ctr.image.toLowerCase().includes(q) ||
        ctr.revision.toLowerCase().includes(q)

      const matchesApp = application === 'all' || ctr.application === application
      const matchesServer = server === 'all' || ctr.server === server
      const matchesStatus = status === 'all' || ctr.status === status

      return matchesQuery && matchesApp && matchesServer && matchesStatus
    })
  }, [containers, query, application, server, status])

  const hasFilters = query.trim() !== '' || application !== 'all' || server !== 'all' || status !== 'all'

  function clearFilters() {
    setQuery('')
    setApplication('all')
    setServer('all')
    setStatus('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search containers by name, app, server, image…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <>
              <Select value={application} onValueChange={(v) => setApplication(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by application">
                  <SelectValue placeholder="Application" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All applications</SelectItem>
                  {availableApplications.map((app) => (
                    <SelectItem key={app} value={app}>
                      {app}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={server} onValueChange={(v) => setServer(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by server">
                  <SelectValue placeholder="Server" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All servers</SelectItem>
                  {availableServers.map((srv) => (
                    <SelectItem key={srv} value={srv}>
                      {srv}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={status} onValueChange={(v) => setStatus(v ?? 'all')}>
                <SelectTrigger className="w-36" aria-label="Filter by state">
                  <SelectValue placeholder="State" />
                </SelectTrigger>
                <SelectContent>
                  {STATE_FILTERS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          }
        />
      }
    >
      {filtered.length === 0 ? (
        <div className="p-8">
          <EmptyState
            icon={Box}
            title={hasFilters ? 'No containers match your filters' : 'No containers found'}
            description={
              hasFilters
                ? 'Try broadening your search term or clearing active filters.'
                : 'Containers deployed across your application fleet will appear here.'
            }
            action={
              hasFilters ? (
                <Button size="sm" variant="outline" onClick={clearFilters}>
                  Clear filters
                </Button>
              ) : undefined
            }
            className="border-0"
          />
        </div>
      ) : (
        <div className="overflow-x-auto">
          <ContainersTable containers={filtered} onActionSuccess={onActionSuccess} />
        </div>
      )}
    </DataTable>
  )
}
