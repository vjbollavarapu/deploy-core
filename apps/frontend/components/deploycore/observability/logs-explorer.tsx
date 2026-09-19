'use client'

import { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { FilterBar } from '@/components/platform/filter-bar'
import { applications } from '@/lib/mock-data'
import {
  filterFleetLogs,
  getFleetLogLines,
  listLogContainers,
  listLogEnvironments,
  listLogRevisions,
  type LogExplorerFilters,
} from '@/lib/observability'

const fleetLines = getFleetLogLines(36)

export function LogsExplorer() {
  const [filters, setFilters] = useState<LogExplorerFilters>({
    application: 'all',
    environment: 'all',
    revision: 'all',
    container: 'all',
    source: 'all',
  })

  const environments = listLogEnvironments()
  const revisions = listLogRevisions(
    filters.application === 'all' ? undefined : filters.application,
  )
  const containerNames = listLogContainers({
    application: filters.application,
    environment: filters.environment,
    revision: filters.revision,
  })
  const lines = filterFleetLogs(fleetLines, filters)

  function update<K extends keyof LogExplorerFilters>(key: K, value: LogExplorerFilters[K]) {
    setFilters((prev) => {
      const next = { ...prev, [key]: value }
      if (key === 'application') {
        next.revision = 'all'
        next.container = 'all'
      }
      if (key === 'environment' || key === 'revision') {
        next.container = 'all'
      }
      return next
    })
  }

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardContent className="pt-4">
          <FilterBar>
            <Select value={filters.source} onValueChange={(v) => update('source', (v as LogExplorerFilters['source']) ?? 'all')}>
              <SelectTrigger className="w-40" aria-label="Filter by source">
                <SelectValue placeholder="Source" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All sources</SelectItem>
                <SelectItem value="application">Applications</SelectItem>
                <SelectItem value="server">Servers</SelectItem>
                <SelectItem value="database">Databases</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={filters.application}
              onValueChange={(v) => update('application', v ?? 'all')}
            >
              <SelectTrigger className="w-48" aria-label="Filter by application">
                <SelectValue placeholder="Application" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All applications</SelectItem>
                {applications.map((app) => (
                  <SelectItem key={app.id} value={app.name}>
                    {app.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={filters.environment}
              onValueChange={(v) => update('environment', v ?? 'all')}
            >
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
            <Select value={filters.revision} onValueChange={(v) => update('revision', v ?? 'all')}>
              <SelectTrigger className="w-36" aria-label="Filter by revision">
                <SelectValue placeholder="Revision" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All revisions</SelectItem>
                {revisions.map((revision) => (
                  <SelectItem key={revision} value={revision}>
                    {revision}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={filters.container} onValueChange={(v) => update('container', v ?? 'all')}>
              <SelectTrigger className="w-52" aria-label="Filter by container">
                <SelectValue placeholder="Container" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All containers</SelectItem>
                {containerNames.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FilterBar>
          <p className="mt-3 text-xs text-muted-foreground">
            Showing {lines.length} lines · filter by application, environment, revision, and container.
            Viewer supports search, severity, timestamps, pause, follow, and download.
          </p>
        </CardContent>
      </Card>
      <BuildLogViewer lines={lines} title="fleet-logs" streaming />
    </div>
  )
}
