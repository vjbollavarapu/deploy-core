'use client'

import { useMemo, useState } from 'react'
import { Server as ServerIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ServersTable } from './servers-table'
import { SERVER_PROVIDERS, SERVER_STATUS_FILTERS } from '@/lib/servers'
import type { Server } from '@/lib/types'

interface ServersFilterTableProps {
  servers: Server[]
}

export function ServersFilterTable({ servers }: ServersFilterTableProps) {
  const [query, setQuery] = useState('')
  const [provider, setProvider] = useState('all')
  const [status, setStatus] = useState('all')

  const availableProviders = useMemo(() => {
    const fromData = Array.from(new Set(servers.map((s) => s.provider).filter(Boolean)))
    const combined = Array.from(new Set([...fromData, ...SERVER_PROVIDERS]))
    return combined
  }, [servers])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return servers.filter((server) => {
      const matchesQuery =
        q === '' ||
        server.name.toLowerCase().includes(q) ||
        (server.ip?.toLowerCase().includes(q) ?? false) ||
        (server.region?.toLowerCase().includes(q) ?? false) ||
        (server.agentVersion?.toLowerCase().includes(q) ?? false)

      const matchesProvider = provider === 'all' || server.provider === provider
      const matchesStatus = status === 'all' || server.status === status

      return matchesQuery && matchesProvider && matchesStatus
    })
  }, [servers, query, provider, status])

  const hasFilters = query.trim() !== '' || provider !== 'all' || status !== 'all'

  function clearFilters() {
    setQuery('')
    setProvider('all')
    setStatus('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search servers by name, IP, region, agent…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <>
              <Select value={provider} onValueChange={(v) => setProvider(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by provider">
                  <SelectValue placeholder="Provider" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All providers</SelectItem>
                  {availableProviders.map((p) => (
                    <SelectItem key={p} value={p}>
                      {p}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={status} onValueChange={(v) => setStatus(v ?? 'all')}>
                <SelectTrigger className="w-36" aria-label="Filter by status">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  {SERVER_STATUS_FILTERS.map((s) => (
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
            icon={ServerIcon}
            title={hasFilters ? 'No servers match your filters' : 'No servers configured'}
            description={
              hasFilters
                ? 'Try broadening your search term or resetting active filters.'
                : 'Register your first host using the Add server button above.'
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
          <ServersTable servers={filtered} />
        </div>
      )}
    </DataTable>
  )
}
