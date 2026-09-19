import Link from 'next/link'
import { notFound } from 'next/navigation'
import {
  Braces,
  GitBranch,
  GitCommitVertical,
  Globe,
  HardDrive,
  KeyRound,
  Network,
} from 'lucide-react'
import { DetailList } from '@/components/platform/detail-list'
import { EmptyState } from '@/components/platform/empty-state'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { StatusBadge } from '@/components/platform/status-badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ApplicationMetricsPanel } from '@/components/deploycore/applications/application-metrics'
import { DeploymentsTable } from '@/components/deploycore/deployments/deployments-table'
import { DomainsTable } from '@/components/deploycore/domains/domains-table'
import { EnvironmentVariablesEditor } from '@/components/deploycore/environment-variables/environment-variables-editor'
import { RevisionsTable } from '@/components/deploycore/revisions/revisions-table'
import {
  findApplication,
  getApplicationDeployments,
  getApplicationDomains,
  getApplicationLogs,
  getApplicationNetworks,
  getApplicationRevisions,
  getApplicationSecrets,
  getApplicationVariables,
  getApplicationVolumes,
} from '@/lib/applications'

export default async function ApplicationSectionPage({
  params,
}: {
  params: Promise<{ applicationId: string; section: string }>
}) {
  const { applicationId, section } = await params
  const application = findApplication(applicationId)
  if (!application) notFound()

  if (section === 'deployments') {
    const rows = getApplicationDeployments(application)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={GitBranch}
                title="No deployments"
                description="Deployment history for this application will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <DeploymentsTable deployments={rows} />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'revisions') {
    const rows = getApplicationRevisions(application)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={GitCommitVertical}
                title="No revisions"
                description="Revision traffic splits and rollbacks will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <RevisionsTable revisions={rows} showApplication={false} />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'logs') {
    return (
      <BuildLogViewer
        lines={getApplicationLogs(application, 120)}
        title={`${application.name}-logs`}
        streaming={application.status !== 'stopped' && application.status !== 'failed'}
      />
    )
  }

  if (section === 'metrics') {
    return <ApplicationMetricsPanel application={application} />
  }

  if (section === 'domains') {
    const rows = getApplicationDomains(application)
    return (
      <Card size="sm">
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Globe}
                title="No domains"
                description="Custom domains and TLS certificates for this application will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <DomainsTable domains={rows} />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'environment') {
    const variables = getApplicationVariables(application)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Environment variables</CardTitle>
          <CardDescription>
            Hierarchy: Organization → Project → Environment → Application. Inherited, overridden, and
            application-specific values.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {variables.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Braces}
                title="No variables"
                description="Environment variables scoped to this application will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <EnvironmentVariablesEditor initialVariables={variables} />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'secrets') {
    const rows = getApplicationSecrets(application)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Secrets metadata</CardTitle>
          <CardDescription>
            Secret references mounted by this application. Values are never displayed or repopulated.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {rows.length === 0 ? (
            <EmptyState
              icon={KeyRound}
              title="No secrets"
              description="Secrets referenced by this application will appear here."
              className="border-0"
            />
          ) : (
            <DetailList
              columns={2}
              items={rows.map((secret) => ({
                label: secret.name,
                value: `${secret.scope} · access ${secret.accessRoles.join('/')} · rotated ${secret.lastRotatedAt}`,
              }))}
            />
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'networking') {
    const rows = getApplicationNetworks(application)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Networking</CardTitle>
          <CardDescription>Networks attached in {application.environment}.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={Network}
                title="No networks"
                description="Ports, ingress, and network attachments will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Network</TableHead>
                  <TableHead>Driver</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Services</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((network) => (
                  <TableRow key={network.id}>
                    <TableCell className="font-medium">{network.name}</TableCell>
                    <TableCell className="text-muted-foreground">{network.driver}</TableCell>
                    <TableCell className="text-muted-foreground">{network.scope}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {network.connectedServices.join(', ') || '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'volumes') {
    const rows = getApplicationVolumes(application)
    return (
      <Card size="sm">
        <CardHeader>
          <CardTitle>Volumes</CardTitle>
          <CardDescription>Persistent volumes attached to this application.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={HardDrive}
                title="No volumes"
                description="Persistent volumes attached to this application will appear here."
                className="border-0"
              />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Volume</TableHead>
                  <TableHead>Mount</TableHead>
                  <TableHead>Usage</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((volume) => (
                  <TableRow key={volume.id}>
                    <TableCell className="font-medium">{volume.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {volume.mountPath}
                    </TableCell>
                    <TableCell className="tabular text-muted-foreground">
                      {volume.usedGb}/{volume.totalGb} GB
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={volume.status} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    )
  }

  if (section === 'settings') {
    return (
      <div className="flex flex-col gap-4">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Application settings</CardTitle>
            <CardDescription>Identity and runtime configuration for {application.name}.</CardDescription>
          </CardHeader>
          <CardContent>
            <DetailList
              columns={2}
              items={[
                { label: 'Name', value: application.name },
                { label: 'Runtime', value: application.runtime },
                {
                  label: 'Project',
                  value: (
                    <Link href="/projects" className="hover:underline">
                      {application.project}
                    </Link>
                  ),
                },
                { label: 'Environment', value: application.environment },
                { label: 'Server', value: application.server },
                { label: 'Instances', value: String(application.instances) },
                { label: 'Repository', value: application.repo },
                { label: 'Branch', value: application.branch },
              ]}
            />
          </CardContent>
        </Card>
        <Card size="sm" className="border-critical/30">
          <CardHeader>
            <CardTitle className="text-critical">Danger zone</CardTitle>
            <CardDescription>
              Destructive actions for this application require confirmation and will be wired to the API later.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  notFound()
}
