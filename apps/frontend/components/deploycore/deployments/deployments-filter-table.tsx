'use client'

import { Rocket } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { cn } from '@/lib/utils'
import {
  DEPLOYMENT_FILTERS,
  matchesDeploymentFilter,
  type DeploymentFilterId,
} from '@/lib/deployments'
import type { Deployment } from '@/lib/types'
import { DeploymentsTable } from './deployments-table'

interface DeploymentsFilterTableProps {
  deployments: Deployment[]
}

export function DeploymentsFilterTable({ deployments }: DeploymentsFilterTableProps) {
  const [query, setQuery] = useState('')
  const [environment, setEnvironment] = useState('all')
  const [filter, setFilter] = useState<DeploymentFilterId>('all')

  const environments = useMemo(
    () => Array.from(new Set(deployments.map((d) => d.environment))),
    [deployments],
  )

  const filterCounts = useMemo(() => {
    const counts = Object.fromEntries(DEPLOYMENT_FILTERS.map((f) => [f.id, 0])) as Record<
      DeploymentFilterId,
      number
    >
    for (const dep of deployments) {
      for (const f of DEPLOYMENT_FILTERS) {
        if (matchesDeploymentFilter(dep, f.id)) counts[f.id] += 1
      }
    }
    return counts
  }, [deployments])

  const filtered = useMemo(
    () =>
      deployments.filter((dep) => {
        const matchesQuery =
          query.trim() === '' ||
          dep.application.toLowerCase().includes(query.toLowerCase()) ||
          dep.commit.toLowerCase().includes(query.toLowerCase()) ||
          dep.commitMessage.toLowerCase().includes(query.toLowerCase()) ||
          String(dep.number).includes(query.trim())
        const matchesEnvironment = environment === 'all' || dep.environment === environment
        return matchesQuery && matchesEnvironment && matchesDeploymentFilter(dep, filter)
      }),
    [deployments, query, environment, filter],
  )

  return (
    <DataTable
      toolbar={
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap gap-1.5" role="tablist" aria-label="Filter by deployment status">
            {DEPLOYMENT_FILTERS.map((item) => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={filter === item.id}
                onClick={() => setFilter(item.id)}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors',
                  filter === item.id
                    ? 'border-primary bg-primary/10 text-primary'
                    : 'border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                )}
              >
                {item.label}
                <span className="tabular text-[10px] opacity-70">{filterCounts[item.id]}</span>
              </button>
            ))}
          </div>

          <DataTableToolbar
            searchPlaceholder="Search deployments..."
            searchValue={query}
            onSearchChange={setQuery}
            filters={
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
            }
            actions={
              <span className="text-xs text-muted-foreground tabular">
                {filtered.length} of {deployments.length} deployments
              </span>
            }
          />
        </div>
      }
    >
      {filtered.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={Rocket}
            title="No deployments found"
            description="Try adjusting your search or filters."
            className="border-0"
          />
        </div>
      ) : (
        <DeploymentsTable deployments={filtered} />
      )}
    </DataTable>
  )
}
