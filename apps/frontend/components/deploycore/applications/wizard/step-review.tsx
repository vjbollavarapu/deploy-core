'use client'

import { DetailList } from '@/components/platform/detail-list'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { projects as rawMockProjects, servers as rawMockServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const mockProjects = getDemoFixtures(rawMockProjects)
const mockServers = getDemoFixtures(rawMockServers)
import {
  sourceTypeLabel,
  type CreateApplicationValues,
} from '@/lib/validations/application'
import type { PlacementProject, PlacementServer } from './step-placement'
import { Loader2 } from 'lucide-react'

interface StepReviewProps {
  values: CreateApplicationValues
  projects?: PlacementProject[]
  servers?: PlacementServer[]
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="text-right font-medium break-all text-foreground">{value || '—'}</span>
    </div>
  )
}

export function StepReview({ values, projects = [], servers = [] }: StepReviewProps) {
  const project =
    projects.find((item) => item.id === values.projectId) ??
    mockProjects.find((item) => item.id === values.projectId)
  const server =
    servers.find((item) => item.id === values.serverId) ??
    mockServers.find((item) => item.id === values.serverId)

  const environmentName =
    project?.environments.find((env) =>
      typeof env === 'string' ? env === values.environment : env.id === values.environment,
    )
  const envDisplay =
    typeof environmentName === 'string'
      ? environmentName
      : environmentName?.name ?? values.environment

  return (
    <div className="flex flex-col gap-3">
      <Card size="sm">
        <CardHeader>
          <CardTitle>Source</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <SummaryRow label="Type" value={sourceTypeLabel(values.sourceType)} />
          {values.sourceType === 'git' && (
            <>
              <SummaryRow label="Repository" value={values.repository} />
              <SummaryRow label="Branch" value={values.branch} />
              <SummaryRow label="Dockerfile" value={values.dockerfile} />
              <SummaryRow label="Build context" value={values.buildContext} />
            </>
          )}
          {values.sourceType === 'docker-image' && (
            <>
              <SummaryRow label="Image" value={values.image} />
              <SummaryRow label="Tag" value={values.imageTag} />
            </>
          )}
          {values.sourceType === 'docker-compose' && (
            <>
              <SummaryRow label="Repository" value={values.repository} />
              <SummaryRow label="Branch" value={values.branch} />
              <SummaryRow label="Compose file" value={values.composeFile} />
            </>
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Runtime & placement</CardTitle>
        </CardHeader>
        <CardContent>
          <DetailList
            columns={2}
            items={[
              { label: 'Name', value: values.name },
              { label: 'Type', value: values.applicationType },
              { label: 'Project', value: project?.name ?? values.projectId },
              { label: 'Environment', value: envDisplay },
              { label: 'Server', value: server?.name ?? values.serverId },
              { label: 'Port', value: String(values.port) },
              { label: 'CPU', value: `${values.cpu} cores` },
              { label: 'RAM', value: `${values.memoryMb} MiB` },
              { label: 'Restart', value: values.restartPolicy },
              { label: 'Command', value: values.command || 'Image default' },
              { label: 'Entrypoint', value: values.entrypoint || 'Image default' },
              { label: 'Domain', value: values.domain || 'None' },
              {
                label: 'Health check',
                value: `${values.healthCheckPath} :${values.healthCheckPort}`,
              },
            ]}
          />
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 text-sm">
          <SummaryRow
            label="Environment variables"
            value={
              values.envVars.length === 0
                ? 'None'
                : values.envVars.map((item) => item.key).filter(Boolean).join(', ')
            }
          />
          <SummaryRow
            label="Secrets"
            value={
              values.secrets.length === 0
                ? 'None'
                : values.secrets.map((item) => item.name).filter(Boolean).join(', ')
            }
          />
        </CardContent>
      </Card>
    </div>
  )
}

interface StepDeployProps {
  values: CreateApplicationValues
  pending: boolean
  projects?: PlacementProject[]
  servers?: PlacementServer[]
}

export function StepDeploy({ values, pending, projects = [], servers = [] }: StepDeployProps) {
  const project =
    projects.find((item) => item.id === values.projectId) ??
    mockProjects.find((item) => item.id === values.projectId)
  const server =
    servers.find((item) => item.id === values.serverId) ??
    mockServers.find((item) => item.id === values.serverId)

  const environmentName =
    project?.environments.find((env) =>
      typeof env === 'string' ? env === values.environment : env.id === values.environment,
    )
  const envDisplay =
    typeof environmentName === 'string'
      ? environmentName
      : environmentName?.name ?? values.environment

  const sourceRef =
    values.sourceType === 'git'
      ? `${values.repository}:${values.branch}`
      : values.sourceType === 'docker-image'
        ? `${values.image}:${values.imageTag}`
        : `${values.repository} (${values.composeFile})`

  return (
    <div className="flex flex-col gap-3">
      <div className="rounded-lg border border-border bg-muted/20 p-4">
        <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
          Pre-flight deployment checklist
        </h4>
        <div className="grid gap-2 text-xs">
          <div className="flex items-center justify-between py-1 border-b border-border/50">
            <span className="text-muted-foreground">Workload</span>
            <span className="font-mono font-medium text-foreground">{values.name}</span>
          </div>
          <div className="flex items-center justify-between py-1 border-b border-border/50">
            <span className="text-muted-foreground">Type & sizing</span>
            <span className="text-foreground">
              {values.applicationType} · {values.cpu} cores · {values.memoryMb} MiB
            </span>
          </div>
          <div className="flex items-center justify-between py-1 border-b border-border/50">
            <span className="text-muted-foreground">Source</span>
            <span className="font-mono text-foreground truncate max-w-[16rem]">{sourceRef}</span>
          </div>
          <div className="flex items-center justify-between py-1 border-b border-border/50">
            <span className="text-muted-foreground">Placement</span>
            <span className="text-foreground">
              {project?.name ?? values.projectId} / {envDisplay} on {server?.name ?? values.serverId}
            </span>
          </div>
          <div className="flex items-center justify-between py-1 border-b border-border/50">
            <span className="text-muted-foreground">Networking</span>
            <span className="text-foreground">
              Port {values.port}
              {values.domain ? ` · ${values.domain}` : ''}
              {` · Health: ${values.healthCheckPath}:${values.healthCheckPort}`}
            </span>
          </div>
        </div>
      </div>

      <div className="flex items-start gap-3 rounded-lg border border-border bg-muted/30 p-4">
        {pending ? (
          <Loader2 className="size-5 shrink-0 animate-spin text-primary mt-0.5" />
        ) : (
          <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/20 text-primary text-xs font-bold">
            ✓
          </span>
        )}
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            {pending
              ? `Creating ${values.name} & starting deployment…`
              : `Ready to deploy ${values.name}`}
          </p>
          <p className="text-xs text-muted-foreground">
            {pending
              ? 'Registering application with control plane and queueing deployment task.'
              : 'Clicking “Deploy” will submit your configuration to the control plane, allocate containers on the target server, and stream the build logs.'}
          </p>
        </div>
      </div>
    </div>
  )
}
