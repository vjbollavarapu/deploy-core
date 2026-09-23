'use client'

import { GitCommitVertical, RotateCcw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { cn } from '@/lib/utils'
import {
  matchesRevisionFilter,
  REVISION_FILTERS,
  type RevisionFilterId,
} from '@/lib/revisions'
import type { Revision } from '@/lib/types'
import { RevisionsTable } from './revisions-table'

interface RevisionsFilterTableProps {
  revisions: Revision[]
  onRevisionUpdated?: () => void
}

export function RevisionsFilterTable({ revisions, onRevisionUpdated }: RevisionsFilterTableProps) {
  const [query, setQuery] = useState('')
  const [applicationId, setApplicationId] = useState('all')
  const [filter, setFilter] = useState<RevisionFilterId>('all')

  const applicationOptions = useMemo(() => {
    const map = new Map<string, string>()
    for (const r of revisions) {
      if (r.applicationId && r.application) {
        map.set(r.applicationId, r.application)
      }
    }
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [revisions])

  const filterCounts = useMemo(() => {
    const counts: Record<RevisionFilterId, number> = {
      all: 0,
      active: 0,
      healthy: 0,
      archived: 0,
    }
    for (const rev of revisions) {
      for (const f of REVISION_FILTERS) {
        if (matchesRevisionFilter(rev, f.id)) counts[f.id] += 1
      }
    }
    return counts
  }, [revisions])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return revisions.filter((rev) => {
      const matchesQuery =
        q === '' ||
        rev.number.toLowerCase().includes(q) ||
        rev.application.toLowerCase().includes(q) ||
        rev.commit.toLowerCase().includes(q) ||
        rev.commitMessage.toLowerCase().includes(q) ||
        rev.imageDigest.toLowerCase().includes(q)
      const matchesApp = applicationId === 'all' || rev.applicationId === applicationId
      return matchesQuery && matchesApp && matchesRevisionFilter(rev, filter)
    })
  }, [revisions, query, applicationId, filter])

  const hasActiveFilters = query.trim() !== '' || applicationId !== 'all' || filter !== 'all'

  function resetFilters() {
    setQuery('')
    setApplicationId('all')
    setFilter('all')
  }

  return (
    <DataTable
      toolbar={
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap gap-1.5" role="tablist" aria-label="Filter by revision status">
            {REVISION_FILTERS.map((item) => (
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
            searchPlaceholder="Search revisions by number, application, commit..."
            searchValue={query}
            onSearchChange={setQuery}
            filters={
              <Select value={applicationId} onValueChange={(v) => setApplicationId(v ?? 'all')}>
                <SelectTrigger className="w-48" aria-label="Filter by application">
                  <SelectValue placeholder="All applications" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All applications</SelectItem>
                  {applicationOptions.map((app) => (
                    <SelectItem key={app.id} value={app.id}>
                      {app.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            }
          />
        </div>
      }
    >
      {filtered.length === 0 ? (
        <div className="py-8">
          <EmptyState
            icon={GitCommitVertical}
            title={hasActiveFilters ? 'No revisions found' : 'No revisions created'}
            description={
              hasActiveFilters
                ? 'No revisions match the selected filters or search terms.'
                : 'Deploy an application to generate immutable revision snapshots with traffic routing.'
            }
            action={
              hasActiveFilters ? (
                <Button variant="outline" size="sm" onClick={resetFilters}>
                  <RotateCcw data-icon="inline-start" />
                  Reset filters
                </Button>
              ) : undefined
            }
          />
        </div>
      ) : (
        <RevisionsTable revisions={filtered} onRevisionUpdated={onRevisionUpdated} />
      )}
    </DataTable>
  )
}
