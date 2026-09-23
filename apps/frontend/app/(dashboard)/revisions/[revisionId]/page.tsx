import { notFound } from 'next/navigation'
import Link from 'next/link'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Badge } from '@/components/ui/badge'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { CodeBlock } from '@/components/platform/code-block'
import { DetailList } from '@/components/platform/detail-list'
import { RevisionDetailActions } from '@/components/deploycore/revisions/revision-detail-actions'
import { revisions as rawRevisions } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const revisions = getDemoFixtures(rawRevisions)
import {
  findRevision,
  formatRevisionNumber,
  getActiveRevision,
  getRevisionsForApplication,
} from '@/lib/revisions'

export default async function RevisionDetailPage({
  params,
}: {
  params: Promise<{ revisionId: string }>
}) {
  const { revisionId } = await params
  const revision = findRevision(revisionId, revisions)
  if (!revision) notFound()

  const siblings = getRevisionsForApplication(revision.applicationId, revisions)
  const active = getActiveRevision(siblings)

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={`Revision ${revision.number}`}
        description={revision.commitMessage}
        breadcrumbs={[
          { label: 'Revisions', href: '/revisions' },
          { label: revision.application, href: `/applications/${revision.applicationId}` },
          { label: revision.number },
        ]}
        badges={
          <>
            <StatusBadge status={revision.status} />
            <EnvironmentBadge environment={revision.environment} />
            {revision.archived ? <Badge variant="secondary">Archived</Badge> : null}
            {revision.traffic > 0 ? (
              <Badge variant="outline">{revision.traffic}% traffic</Badge>
            ) : null}
          </>
        }
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <Link
              href={`/applications/${revision.applicationId}`}
              className="hover:text-foreground hover:underline"
            >
              {revision.application}
            </Link>
            <span className="font-mono">{revision.commit}</span>
            <span>{revision.runtime}</span>
            <span>by {revision.createdBy.name}</span>
            <span>{revision.createdAt}</span>
          </div>
        }
        actions={
          <RevisionDetailActions revision={revision} active={active} compareWithId={active?.id} />
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Snapshot</CardTitle>
            <CardDescription>
              Immutable configuration for revision {formatRevisionNumber(revision.number)}. Secret
              values are never displayed.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <DetailList
              columns={2}
              items={[
                { label: 'Runtime', value: revision.runtime },
                { label: 'Server', value: revision.server },
                { label: 'CPU', value: `${revision.cpuLimit} vCPU` },
                { label: 'RAM', value: `${revision.memoryLimit} MB` },
                {
                  label: 'Health check',
                  value: `${revision.healthCheck.type} ${revision.healthCheck.path} · ${revision.healthCheck.interval}`,
                },
                {
                  label: 'Domains',
                  value: revision.domains.length ? revision.domains.join(', ') : '—',
                },
                {
                  label: 'Volumes',
                  value: revision.volumes.length ? revision.volumes.join(', ') : '—',
                },
                {
                  label: 'Traffic',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <Progress value={revision.traffic} className="w-24 flex-none gap-0" />
                      <span className="font-mono text-xs">{revision.traffic}%</span>
                    </span>
                  ),
                },
              ]}
            />
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">Command</span>
              <CodeBlock code={revision.command} />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">Image</span>
              <CodeBlock code={revision.image} />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">Image digest</span>
              <CodeBlock code={revision.imageDigest} />
            </div>
          </CardContent>
        </Card>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Environment variables</CardTitle>
              <CardDescription>Metadata only — values are not stored on revisions.</CardDescription>
            </CardHeader>
            <CardContent>
              <ul className="divide-y divide-border rounded-lg border border-border">
                {revision.envVars.map((env) => (
                  <li
                    key={`${env.key}-${env.scope}`}
                    className="flex items-center justify-between gap-2 px-3 py-2 text-sm"
                  >
                    <span className="font-mono text-xs">{env.key}</span>
                    <span className="text-xs text-muted-foreground">
                      {env.scope}
                      {env.secret ? ' · secret ref' : ''}
                    </span>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Secret references</CardTitle>
              <CardDescription>Names only. Contents are never revealed.</CardDescription>
            </CardHeader>
            <CardContent>
              {revision.secretRefs.length === 0 ? (
                <p className="text-sm text-muted-foreground">No secret references.</p>
              ) : (
                <ul className="flex flex-col gap-1.5">
                  {revision.secretRefs.map((name) => (
                    <li
                      key={name}
                      className="flex items-center justify-between rounded-md border border-border px-2.5 py-1.5"
                    >
                      <span className="font-mono text-xs">{name}</span>
                      <span className="font-mono text-[11px] text-muted-foreground">••••••••</span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  )
}
