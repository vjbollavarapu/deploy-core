'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
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
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { apiClient, type Application as WireApplication, type Page } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { applications as rawApplications } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import {
  filterFleetLogs,
  getFleetLogLines,
  listLogContainers,
  listLogEnvironments,
  listLogRevisions,
  type LogExplorerFilters,
} from '@/lib/observability'
import type { LogLine } from '@/lib/types'

interface WireLogEntry {
  cursor?: string
  timestamp?: string
  applicationId?: string
  kind?: string
  stream?: string
  message?: string
  sequence?: number
}

function parseLogLevel(stream?: string, message?: string): LogLine['level'] {
  const msg = (message || '').toLowerCase()
  if (stream === 'stderr' || msg.includes('err') || msg.includes('fail') || msg.includes('fatal')) {
    return 'error'
  }
  if (msg.includes('warn')) {
    return 'warn'
  }
  if (msg.includes('debug')) {
    return 'debug'
  }
  return 'info'
}

function formatLogTimestamp(raw?: string): string {
  if (!raw) return new Date().toLocaleTimeString()
  try {
    const d = new Date(raw)
    return isNaN(d.getTime()) ? raw : d.toLocaleTimeString()
  } catch {
    return raw
  }
}

export function LogsExplorer() {
  const { activeOrg } = useOrganization()
  const demoApps = getDemoFixtures(rawApplications)

  const [availableApps, setAvailableApps] = useState<Array<{ id: string; name: string }>>(
    () => demoApps.map((a) => ({ id: a.id, name: a.name })),
  )
  const [filters, setFilters] = useState<LogExplorerFilters>({
    application: 'all',
    environment: 'all',
    revision: 'all',
    container: 'all',
    source: 'all',
  })

  const [apiLines, setApiLines] = useState<LogLine[]>([])
  const [isLoadingLogs, setIsLoadingLogs] = useState(false)
  const [logError, setLogError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  // Load real applications from the control plane
  useEffect(() => {
    let cancelled = false

    async function loadApps() {
      if (!activeOrg?.id) return
      try {
        const res = await apiClient.get<Page<WireApplication>>(
          `/applications?organizationId=${activeOrg.id}`,
        )
        if (cancelled) return
        if (Array.isArray(res?.items) && res.items.length > 0) {
          const mapped = res.items.map((app) => ({
            id: app.id || app.slug || '',
            name: app.name || 'Untitled',
          }))
          setAvailableApps(mapped)
        } else if (isDemoModeEnabled()) {
          setAvailableApps(demoApps.map((a) => ({ id: a.id, name: a.name })))
        } else {
          setAvailableApps([])
        }
      } catch {
        if (!cancelled && isDemoModeEnabled()) {
          setAvailableApps(demoApps.map((a) => ({ id: a.id, name: a.name })))
        }
      }
    }

    void loadApps()

    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, demoApps])

  // Load logs for the selected application if real mode
  useEffect(() => {
    let cancelled = false

    async function fetchSelectedLogs() {
      if (filters.application === 'all') {
        setApiLines([])
        return
      }

      const targetApp = availableApps.find((a) => a.name === filters.application)
      if (!targetApp || !targetApp.id) return

      setIsLoadingLogs(true)
      setLogError(null)

      try {
        const res = await apiClient.get<{ entries?: WireLogEntry[] }>(
          `/applications/${targetApp.id}/logs?follow=false`,
        )

        if (cancelled) return

        if (Array.isArray(res?.entries) && res.entries.length > 0) {
          const mapped: LogLine[] = res.entries.map((e, index) => ({
            id: e.cursor || `${e.sequence ?? index}-${index}`,
            timestamp: formatLogTimestamp(e.timestamp),
            level: parseLogLevel(e.stream, e.message),
            container: targetApp.name,
            application: targetApp.name,
            environment: filters.environment === 'all' ? 'production' : filters.environment,
            revision: filters.revision === 'all' ? 'rev-latest' : filters.revision,
            message: e.message || '',
          }))
          setApiLines(mapped)
        } else {
          setApiLines([])
        }
      } catch (err) {
        if (cancelled) return
        if (!isDemoModeEnabled()) {
          setLogError(err instanceof Error ? err.message : 'Failed to fetch logs')
        }
      } finally {
        if (!cancelled) {
          setIsLoadingLogs(false)
        }
      }
    }

    void fetchSelectedLogs()

    return () => {
      cancelled = true
    }
  }, [filters.application, filters.environment, filters.revision, availableApps, refreshKey])

  const demoFleetLines = useMemo(() => getFleetLogLines(36), [])

  const combinedLines = useMemo(() => {
    if (filters.application !== 'all' && apiLines.length > 0) {
      return apiLines
    }
    if (isDemoModeEnabled()) {
      return demoFleetLines
    }
    return apiLines
  }, [filters.application, apiLines, demoFleetLines])

  const environments = useMemo(() => listLogEnvironments(), [])
  const revisions = useMemo(
    () => listLogRevisions(filters.application === 'all' ? undefined : filters.application),
    [filters.application],
  )
  const containerNames = useMemo(
    () =>
      listLogContainers({
        application: filters.application,
        environment: filters.environment,
        revision: filters.revision,
      }),
    [filters.application, filters.environment, filters.revision],
  )

  const lines = useMemo(
    () => filterFleetLogs(combinedLines, filters),
    [combinedLines, filters],
  )

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

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardContent className="pt-4">
          <FilterBar>
            <Select
              value={filters.source}
              onValueChange={(v) =>
                update('source', (v as LogExplorerFilters['source']) ?? 'all')
              }
            >
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
                {availableApps.map((app) => (
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
            <Select
              value={filters.revision}
              onValueChange={(v) => update('revision', v ?? 'all')}
            >
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
            <Select
              value={filters.container}
              onValueChange={(v) => update('container', v ?? 'all')}
            >
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
            Showing {lines.length} lines · filter by application, environment, revision, and
            container. Viewer supports search, severity, timestamps, pause, follow, and download.
          </p>
        </CardContent>
      </Card>

      {isLoadingLogs && lines.length === 0 ? (
        <LoadingState label="Fetching logs from control plane…" />
      ) : logError && lines.length === 0 ? (
        <ErrorState
          title="Could not load logs"
          message={logError}
          onRetry={reload}
        />
      ) : (
        <BuildLogViewer lines={lines} title="fleet-logs" streaming />
      )}
    </div>
  )
}
