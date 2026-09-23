import Link from 'next/link'
import { notFound } from 'next/navigation'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { RevisionCompare } from '@/components/deploycore/revisions/revision-compare'
import { revisions as rawRevisions } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const revisions = getDemoFixtures(rawRevisions)
import { findRevision, formatRevisionNumber } from '@/lib/revisions'

export default async function RevisionComparePage({
  searchParams,
}: {
  searchParams: Promise<{ left?: string; right?: string }>
}) {
  const { left: leftId, right: rightId } = await searchParams
  const left = leftId ? findRevision(leftId, revisions) : undefined
  const right = rightId ? findRevision(rightId, revisions) : undefined

  if (!left || !right) notFound()

  const sameApp = left.applicationId === right.applicationId

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title="Compare revisions"
        description={
          sameApp
            ? `${left.application} · ${formatRevisionNumber(left.number)} vs ${formatRevisionNumber(right.number)}`
            : 'Selected revisions belong to different applications.'
        }
        breadcrumbs={[
          { label: 'Revisions', href: '/revisions' },
          { label: 'Compare' },
        ]}
        badges={
          <>
            <StatusBadge status={left.status} />
            <StatusBadge status={right.status} />
          </>
        }
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<Link href={`/revisions/${left.id}`} />}
            >
              View {formatRevisionNumber(left.number)}
            </Button>
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<Link href={`/revisions/${right.id}`} />}
            >
              View {formatRevisionNumber(right.number)}
            </Button>
          </div>
        }
      />

      {!sameApp ? (
        <Card>
          <CardHeader>
            <CardTitle>Cross-application compare</CardTitle>
            <CardDescription>
              Comparing revisions from {left.application} and {right.application}. Field diffs are
              still shown; rollbacks only apply within the same application.
            </CardDescription>
          </CardHeader>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Configuration diff</CardTitle>
          <CardDescription>
            Commit, image, resources, command, env metadata, secret references, volumes, health
            check, and domains. Secret contents are never included.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <RevisionCompare left={left} right={right} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
