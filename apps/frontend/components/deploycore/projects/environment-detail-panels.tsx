import Link from 'next/link'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { EmptyState } from '@/components/platform/empty-state'
import { MetricCard } from '@/components/platform/metric-card'
import { StatusBadge } from '@/components/platform/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type {
  Application,
  DatabaseInstance,
  DomainRecord,
  EnvVarEntry,
  SecretItem,
  Status,
} from '@/lib/types'
import { Activity, Database, Globe, KeyRound, Variable } from 'lucide-react'

interface EnvironmentHealthSummaryProps {
  health: Status
  applicationCount: number
  databaseCount: number
  domainCount: number
  variableCount: number
  secretCount: number
}

export function EnvironmentHealthSummary({
  health,
  applicationCount,
  databaseCount,
  domainCount,
  variableCount,
  secretCount,
}: EnvironmentHealthSummaryProps) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
      <MetricCard
        label="Health"
        value={health === 'healthy' ? 'Healthy' : health === 'degraded' ? 'Degraded' : health === 'failed' ? 'Failed' : 'Unknown'}
        tone={health === 'failed' ? 'critical' : health === 'degraded' ? 'warning' : 'default'}
        icon={Activity}
      />
      <MetricCard label="Applications" value={String(applicationCount)} />
      <MetricCard label="Databases" value={String(databaseCount)} icon={Database} />
      <MetricCard label="Domains" value={String(domainCount)} icon={Globe} />
      <MetricCard label="Variables" value={String(variableCount)} icon={Variable} />
      <MetricCard label="Secrets" value={String(secretCount)} icon={KeyRound} />
    </div>
  )
}

export function EnvironmentVariablesPanel({ variables }: { variables: EnvVarEntry[] }) {
  if (variables.length === 0) {
    return (
      <EmptyState
        icon={Variable}
        title="No environment variables"
        description="Project and environment scoped variables will appear here."
      />
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Variables</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Key</TableHead>
              <TableHead>Scope</TableHead>
              <TableHead>Value</TableHead>
              <TableHead>Source</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {variables.map((variable) => (
              <TableRow key={variable.id}>
                <TableCell className="font-mono text-xs">{variable.key}</TableCell>
                <TableCell className="text-muted-foreground">{variable.scope}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {variable.secret ? '••••••••' : variable.value}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {variable.source}
                  {variable.overridden ? ' · overridden' : ''}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

export function EnvironmentSecretsPanel({ secrets }: { secrets: SecretItem[] }) {
  if (secrets.length === 0) {
    return (
      <EmptyState
        icon={KeyRound}
        title="No secrets metadata"
        description="Secret references for this environment will appear here. Values are never shown."
      />
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Secrets metadata</CardTitle>
      </CardHeader>
      <CardContent>
        <DetailList
          columns={2}
          items={secrets.map((secret) => ({
            label: secret.name,
            value: `${secret.scope} · ${secret.applications.join(', ') || 'unassigned'} · updated ${secret.updatedAt}`,
          }))}
        />
      </CardContent>
    </Card>
  )
}

export function EnvironmentDomainsPanel({ domains }: { domains: DomainRecord[] }) {
  if (domains.length === 0) {
    return (
      <EmptyState
        icon={Globe}
        title="No domains"
        description="Custom domains routed to applications in this environment will appear here."
      />
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Domains</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Domain</TableHead>
              <TableHead>Application</TableHead>
              <TableHead>TLS</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {domains.map((domain) => (
              <TableRow key={domain.id}>
                <TableCell className="font-mono text-xs">{domain.domain}</TableCell>
                <TableCell>
                  <Link href={`/applications/${domain.applicationId}`} className="hover:underline">
                    {domain.application}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {domain.https ? `expires ${domain.certExpiry}` : 'HTTP only'}
                </TableCell>
                <TableCell>
                  <StatusBadge status={domain.status} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

export function EnvironmentApplicationsHealth({ applications }: { applications: Application[] }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Application health</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        {applications.length === 0 ? (
          <p className="px-4 pb-4 text-sm text-muted-foreground">No applications in this environment.</p>
        ) : (
          <ul className="divide-y divide-border">
            {applications.map((app) => (
              <li key={app.id} className="flex items-center justify-between gap-3 px-4 py-2.5">
                <div className="min-w-0">
                  <Link href={`/applications/${app.id}`} className="text-sm font-medium hover:underline">
                    {app.name}
                  </Link>
                  <p className="text-xs text-muted-foreground">
                    {app.runtime} · {app.instances} instance{app.instances === 1 ? '' : 's'} · uptime{' '}
                    {app.uptime}
                  </p>
                </div>
                <StatusBadge status={app.status} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function EnvironmentDatabasesPanel({ databases }: { databases: DatabaseInstance[] }) {
  if (databases.length === 0) {
    return (
      <EmptyState
        icon={Database}
        title="No databases"
        description="Database instances assigned to this environment will appear here."
      />
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Databases</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Instance</TableHead>
              <TableHead>Engine</TableHead>
              <TableHead>Server</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {databases.map((db) => (
              <TableRow key={db.id}>
                <TableCell>
                  <Link href={`/databases/${db.id}`} className="hover:underline">
                    {db.name}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {db.type} {db.version}
                </TableCell>
                <TableCell className="text-muted-foreground">{db.server}</TableCell>
                <TableCell>
                  <StatusBadge status={db.status} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
