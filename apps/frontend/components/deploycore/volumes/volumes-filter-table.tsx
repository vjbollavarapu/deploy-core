'use client'

import { useMemo, useState } from 'react'
import { HardDrive } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { VolumesTable } from './volumes-table'
import type { Volume } from '@/lib/types'

interface VolumesFilterTableProps {
  volumes: Volume[]
  onDelete?: () => void
}

export function VolumesFilterTable({ volumes, onDelete }: VolumesFilterTableProps) {
  const [query, setQuery] = useState('')
  const [server, setServer] = useState('all')
  const [driver, setDriver] = useState('all')

  const availableServers = useMemo(
    () => Array.from(new Set(volumes.map((v) => v.server).filter(Boolean))),
    [volumes],
  )

  const availableDrivers = useMemo(
    () => Array.from(new Set(volumes.map((v) => v.driver).filter(Boolean))),
    [volumes],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return volumes.filter((vol) => {
      const matchesQuery =
        q === '' ||
        vol.name.toLowerCase().includes(q) ||
        vol.server.toLowerCase().includes(q) ||
        vol.mountPath.toLowerCase().includes(q) ||
        vol.attachedResource.toLowerCase().includes(q) ||
        vol.driver.toLowerCase().includes(q)

      const matchesServer = server === 'all' || vol.server === server
      const matchesDriver = driver === 'all' || vol.driver === driver

      return matchesQuery && matchesServer && matchesDriver
    })
  }, [volumes, query, server, driver])

  const hasFilters = query.trim() !== '' || server !== 'all' || driver !== 'all'

  function clearFilters() {
    setQuery('')
    setServer('all')
    setDriver('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search volumes by name, server, mount path…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <>
              <Select value={server} onValueChange={(v) => setServer(v ?? 'all')}>
                <SelectTrigger className="w-44" aria-label="Filter by server">
                  <SelectValue placeholder="Server" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All servers</SelectItem>
                  {availableServers.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
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
            icon={HardDrive}
            title={hasFilters ? 'No volumes match your filters' : 'No volumes found'}
            description={
              hasFilters
                ? 'Try broadening your search term or clearing active filters.'
                : 'Persistent storage volumes attached to applications or databases will appear here.'
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
          <VolumesTable volumes={filtered} onDelete={onDelete} />
        </div>
      )}
    </DataTable>
  )
}
