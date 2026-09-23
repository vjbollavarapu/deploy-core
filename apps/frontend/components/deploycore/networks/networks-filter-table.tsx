'use client'

import { useMemo, useState } from 'react'
import { Network as NetworkIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { NetworksTable } from './networks-table'
import type { DockerNetwork } from '@/lib/types'

interface NetworksFilterTableProps {
  networks: DockerNetwork[]
}

export function NetworksFilterTable({ networks }: NetworksFilterTableProps) {
  const [query, setQuery] = useState('')
  const [environment, setEnvironment] = useState('all')
  const [driver, setDriver] = useState('all')

  const availableEnvironments = useMemo(
    () => Array.from(new Set(networks.map((net) => net.environment).filter(Boolean))),
    [networks],
  )

  const availableDrivers = useMemo(
    () => Array.from(new Set(networks.map((net) => net.driver).filter(Boolean))),
    [networks],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return networks.filter((net) => {
      const matchesQuery =
        q === '' ||
        net.name.toLowerCase().includes(q) ||
        net.project.toLowerCase().includes(q) ||
        net.driver.toLowerCase().includes(q) ||
        net.connectedServices.some((s) => s.toLowerCase().includes(q))

      const matchesEnv = environment === 'all' || net.environment === environment
      const matchesDriver = driver === 'all' || net.driver === driver

      return matchesQuery && matchesEnv && matchesDriver
    })
  }, [networks, query, environment, driver])

  const hasFilters = query.trim() !== '' || environment !== 'all' || driver !== 'all'

  function clearFilters() {
    setQuery('')
    setEnvironment('all')
    setDriver('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search networks by name, project, service…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <>
              <Select value={environment} onValueChange={(v) => setEnvironment(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by environment">
                  <SelectValue placeholder="Environment" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All environments</SelectItem>
                  {availableEnvironments.map((env) => (
                    <SelectItem key={env} value={env}>
                      {env}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={driver} onValueChange={(v) => setDriver(v ?? 'all')}>
                <SelectTrigger className="w-36" aria-label="Filter by driver">
                  <SelectValue placeholder="Driver" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All drivers</SelectItem>
                  {availableDrivers.map((d) => (
                    <SelectItem key={d} value={d}>
                      {d}
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
            icon={NetworkIcon}
            title={hasFilters ? 'No networks match your filters' : 'No networks found'}
            description={
              hasFilters
                ? 'Try broadening your search term or clearing active filters.'
                : 'Project overlay and bridge networks will appear here.'
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
          <NetworksTable networks={filtered} />
        </div>
      )}
    </DataTable>
  )
}
