import { notFound } from 'next/navigation'
import Link from 'next/link'
import { GitBranch, GitCommit, RotateCcw, Server, User, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { CodeBlock } from '@/components/platform/code-block'
import { DeploymentPipeline } from '@/components/platform/deployment-pipeline'
import { DeploymentEventTimeline } from '@/components/platform/deployment-event-timeline'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import {
  DEPLOYMENT_FAILURE_LABELS,
  DEPLOYMENT_PHASE_LABELS,
} from '@/lib/deployments'
import { deployments, generateLogLines } from '@/lib/mock-data'

export default async function DeploymentDetailPage({
  params,
}: {
  params: Promise<{ deploymentId: string }>
}) {
  const { deploymentId } = await params
  const deployment = deployments.find((d) => d.id === deploymentId)
  if (!deployment) notFound()

  const logs = generateLogLines(deployment.application, 160)
  const phaseLabel = deployment.failureReason
    ? DEPLOYMENT_FAILURE_LABELS[deployment.failureReason]
    : DEPLOYMENT_PHASE_LABELS[deployment.phase]
  const isActive = deployment.status === 'deploying' || deployment.status === 'queued'

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={`Deployment #${deployment.number}`}
        description={deployment.commitMessage}
        breadcrumbs={[
          { label: 'Deployments', href: '/deployments' },
          { label: `#${deployment.number}` },
        ]}
        badges={
          <>
            <StatusBadge status={deployment.status} />
            <EnvironmentBadge environment={deployment.environment} />
            <span className="rounded-md border border-border bg-muted/50 px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
              {deployment.phase}
            </span>
          </>
        }
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <Link
              href={`/applications/${deployment.applicationId}`}
              className="hover:text-foreground hover:underline"
            >
              {deployment.application}
            </Link>
            <span>{deployment.project}</span>
            <span className="font-mono">{deployment.revision}</span>
            <span>{deployment.startedAt}</span>
            <span>{phaseLabel}</span>
          </div>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {(deployment.status === 'deploying' ||
              deployment.status === 'queued' ||
              deployment.status === 'pending') && (
              <Button size="sm" variant="outline">
                <XCircle data-icon="inline-start" />
                Cancel
              </Button>
            )}
            <Button size="sm">
              <RotateCcw data-icon="inline-start" />
              Redeploy
            </Button>
          </div>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Details</CardTitle>
            <CardDescription>Source and target information for this deployment.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <DetailField label="Application">
                <Link
                  href={`/applications/${deployment.applicationId}`}
                  className="text-foreground hover:underline"
                >
                  {deployment.application}
                </Link>
              </DetailField>
              <DetailField label="Environment">
                <EnvironmentBadge environment={deployment.environment} />
              </DetailField>
              <DetailField label="Revision">
                <span className="font-mono text-foreground">{deployment.revision}</span>
              </DetailField>
              <DetailField label="Duration">
                <span className="text-foreground">{deployment.duration}</span>
              </DetailField>
            </div>
            <Separator />
            <div className="flex flex-col gap-3">
              <div className="flex items-center gap-2 text-sm">
                <GitCommit className="size-4 shrink-0 text-muted-foreground" />
                <span className="font-mono text-foreground">{deployment.commit}</span>
                <span className="text-muted-foreground">{deployment.commitMessage}</span>
              </div>
              <div className="flex items-center gap-2 text-sm">
                <GitBranch className="size-4 shrink-0 text-muted-foreground" />
                <span className="text-foreground">{deployment.repo}</span>
                <span className="text-muted-foreground">@ {deployment.branch}</span>
              </div>
              <div className="flex items-center gap-2 text-sm">
                <Server className="size-4 shrink-0 text-muted-foreground" />
                <span className="text-foreground">{deployment.server}</span>
              </div>
              <div className="flex items-center gap-2 text-sm">
                <User className="size-4 shrink-0 text-muted-foreground" />
                <span className="text-foreground">{deployment.triggeredBy}</span>
                <span className="text-muted-foreground">
                  triggered this deployment {deployment.startedAt}
                </span>
              </div>
            </div>
            <Separator />
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">Image</span>
              <CodeBlock code={deployment.image} />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>State machine</CardTitle>
            <CardDescription>
              {deployment.failureReason
                ? `Failed at ${DEPLOYMENT_FAILURE_LABELS[deployment.failureReason]}`
                : `Current phase: ${DEPLOYMENT_PHASE_LABELS[deployment.phase]}`}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DeploymentPipeline steps={deployment.steps} />
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle>Event timeline</CardTitle>
            <CardDescription>Ordered phase transitions for this run.</CardDescription>
          </CardHeader>
          <CardContent>
            <DeploymentEventTimeline events={deployment.events} />
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Build logs</CardTitle>
            <CardDescription>
              Streaming-ready build and runtime output
              {isActive ? ' (live)' : ''}.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <BuildLogViewer
              lines={logs}
              streaming={isActive}
              title={`${deployment.application}-deploy-${deployment.number}`}
            />
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  )
}

function DetailField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="text-sm">{children}</div>
    </div>
  )
}
