'use client'

import { useCallback, useEffect, useState } from 'react'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { apiClient } from '@/lib/api'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import { getApplicationLogs } from '@/lib/applications'
import type { Application, LogLine } from '@/lib/types'

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

export function ApplicationLogsPanel({ application }: { application: Application }) {
  const [lines, setLines] = useState<LogLine[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  const reload = useCallback(() => {
    setRefreshKey((k) => k + 1)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadLogs() {
      setIsLoading(true)
      setError(null)

      try {
        const res = await apiClient.get<{ entries?: WireLogEntry[] }>(
          `/applications/${application.id}/logs?follow=false`,
        )

        if (cancelled) return

        if (Array.isArray(res?.entries) && res.entries.length > 0) {
          const mapped: LogLine[] = res.entries.map((e, index) => ({
            id: e.cursor || `${e.sequence ?? index}-${index}`,
            timestamp: formatLogTimestamp(e.timestamp),
            level: parseLogLevel(e.stream, e.message),
            container: application.name,
            application: application.name,
            environment: application.environment,
            revision: application.revision,
            message: e.message || '',
          }))
          setLines(mapped)
        } else if (isDemoModeEnabled()) {
          setLines(getApplicationLogs(application, 60))
        } else {
          setLines([])
        }
      } catch (err) {
        if (cancelled) return
        if (isDemoModeEnabled()) {
          setLines(getApplicationLogs(application, 60))
        } else {
          setError(err instanceof Error ? err.message : 'Failed to fetch application logs')
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void loadLogs()

    return () => {
      cancelled = true
    }
  }, [application, refreshKey])

  if (isLoading && lines.length === 0) {
    return <LoadingState label="Loading application logs…" />
  }

  if (error && lines.length === 0) {
    return (
      <ErrorState
        title="Could not load logs"
        message={error}
        onRetry={reload}
      />
    )
  }

  return (
    <BuildLogViewer
      lines={lines}
      streaming={application.status === 'running' || application.status === 'deploying'}
      title={`${application.name}-runtime`}
      showSeverity
    />
  )
}
