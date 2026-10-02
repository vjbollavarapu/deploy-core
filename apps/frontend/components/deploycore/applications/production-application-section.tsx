'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { FileQuestion, GitBranch, Braces } from 'lucide-react'
import { ApplicationLogsPanel } from '@/components/deploycore/applications/application-logs-panel'
import { ApplicationSettingsPanel } from '@/components/deploycore/applications/application-settings-panel'
import { useProductionApplication } from '@/components/deploycore/applications/production-application-shell'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiClient } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import {
  loadProductionApplicationDeployments,
  loadProductionVariables,
  toSettingsApplication,
  type DeploymentDetail,
  type VariableDetail,
} from '@/lib/control-plane/detail-read'

function show(value: string | null | undefined): string {
  return value && value.trim() ? value : '—'
}

function UnavailableSection({ title, description }: { title: string; description: string }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <EmptyState icon={FileQuestion} title="Nothing to show" description={description} className="border-0" />
      </CardContent>
    </Card>
  )
}

export function ProductionApplicationSection({ section }: { section: string }) {
  const application = useProductionApplication()
  if (!application) return null

  if (section === 'deployments') return <DeploymentsSection applicationId={application.id} />
  if (section === 'environment') return <VariablesSection applicationId={application.id} />
  if (section === 'logs') {
    return (
      <ApplicationLogsPanel
        application={{
          id: application.id,
          name: application.name,
          environment: application.environmentName ?? application.environmentId ?? '',
          status: application.status,
        }}
      />
    )
  }
  if (section === 'settings') {
    return (
      <ApplicationSettingsPanel application={toSettingsApplication(application)} />
    )
  }
  if (section === 'networking') {
    return (
      <UnavailableSection
        title="Networking"
        description="The control plane does not expose an application network list. No network attachments are invented here."
      />
    )
  }
  if (section === 'revisions' || section === 'domains' || section === 'secrets' || section === 'metrics') {
    return (
      <UnavailableSection
        title={section[0].toUpperCase() + section.slice(1)}
        description="This section is not loaded from demo data. A production view for it is not available on this page yet."
      />
    )
  }
  return (
    <UnavailableSection
      title="Unknown section"
      description="This application section is not available."
    />
  )
}

function DeploymentsSection({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<DeploymentDetail[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadProductionApplicationDeployments(apiClient, activeOrg.id, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setRows(result.value)
        return
      }
      setError(result.kind === 'error' ? result.message : 'Deployments were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId, attempt])

  if (!activeOrg?.id) {
    return <ErrorState title="Could not load deployments" message="Select an organization to load deployments." />
  }
  if (error) {
    return (
      <ErrorState
        title="Could not load deployments"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((value) => value + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading deployments…" />
  if (rows.length === 0) {
    return <EmptyState icon={GitBranch} title="No deployments" description="This application has no deployments in the control plane." />
  }

  return (
    <Card size="sm">
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Deployment</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Trigger</TableHead>
              <TableHead>Error</TableHead>
              <TableHead>Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell>
                  <Link href={`/deployments/${row.id}`} className="font-mono text-xs hover:underline">
                    {row.id}
                  </Link>
                </TableCell>
                <TableCell>{row.status}</TableCell>
                <TableCell>{show(row.trigger)}</TableCell>
                <TableCell className="max-w-64 truncate">{show(row.errorCode ?? row.errorMessage)}</TableCell>
                <TableCell>{show(row.startedAt ?? row.createdAt)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function VariablesSection({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<VariableDetail[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadProductionVariables(apiClient, activeOrg.id, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setRows(result.value)
        return
      }
      setError(result.kind === 'error' ? result.message : 'Variables were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId, attempt])

  if (!activeOrg?.id) {
    return <ErrorState title="Could not load variables" message="Select an organization to load variables." />
  }
  if (error) {
    return (
      <ErrorState
        title="Could not load variables"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((value) => value + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading variables…" />
  if (rows.length === 0) {
    return <EmptyState icon={Braces} title="No variables" description="This application has no variables in the control plane." />
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Environment variables</CardTitle>
        <CardDescription>Values stored for this application. Demo fixtures are not shown.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Key</TableHead>
              <TableHead>Value</TableHead>
              <TableHead>Scope</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.id || row.key}>
                <TableCell className="font-mono text-xs">{row.key}</TableCell>
                <TableCell className="font-mono text-xs">{row.value}</TableCell>
                <TableCell>{row.scope}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
