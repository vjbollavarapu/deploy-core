'use client'

import { useMemo, useState } from 'react'
import { PackageSearch } from 'lucide-react'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ApplicationsTable } from './applications-table'
import type { Application } from '@/lib/types'
import { STATUS_CONFIG } from '@/lib/status'

interface ApplicationsFilterTableProps {
  applications: Application[]
}

export function ApplicationsFilterTable({ applications }: ApplicationsFilterTableProps) {
  const [query, setQuery] = useState('')
  const [environment, setEnvironment] = useState('all')
  const [status, setStatus] = useState('all')

  const environments = useMemo(
    () => Array.from(new Set(applications.map((a) => a.environment))),
    [applications],
  )
  const statuses = useMemo(() => Array.from(new Set(applications.map((a) => a.status))), [applications])

  const filtered = useMemo(
    () =>
      applications.filter((app) => {
        const q = query.trim().toLowerCase()
        const matchesQuery =
          q === '' ||
          app.name.toLowerCase().includes(q) ||
          app.project.toLowerCase().includes(q) ||
          app.domain.toLowerCase().includes(q) ||
          app.server.toLowerCase().includes(q)
        const matchesEnvironment = environment === 'all' || app.environment === environment
        const matchesStatus = status === 'all' || app.status === status
        return matchesQuery && matchesEnvironment && matchesStatus
      }),
    [applications, query, environment, status],
  )

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search applications…"
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
                  {environments.map((env) => (
                    <SelectItem key={env} value={env}>
                      {env}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={status} onValueChange={(v) => setStatus(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by status">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All statuses</SelectItem>
                  {statuses.map((s) => (
                    <SelectItem key={s} value={s}>
                      {STATUS_CONFIG[s].label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          }
          actions={
            <span className="text-xs text-muted-foreground tabular">
              {filtered.length} of {applications.length}
            </span>
          }
        />
      }
    >
      {filtered.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={PackageSearch}
            title="No applications found"
            description="Try adjusting your search or filters."
            className="border-0"
          />
        </div>
      ) : (
        <ApplicationsTable applications={filtered} listing />
      )}
    </DataTable>
  )
}
