'use client'

import { DetailList } from '@/components/platform/detail-list'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { projects, servers } from '@/lib/mock-data'
import {
  sourceTypeLabel,
  type CreateApplicationValues,
} from '@/lib/validations/application'

interface StepReviewProps {
  values: CreateApplicationValues
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="text-right font-medium break-all text-foreground">{value || '—'}</span>
    </div>
  )
}

export function StepReview({ values }: StepReviewProps) {
  const project = projects.find((item) => item.id === values.projectId)
  const server = servers.find((item) => item.id === values.serverId)

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
              { label: 'Project', value: project?.name ?? '—' },
              { label: 'Environment', value: values.environment },
              { label: 'Server', value: server?.name ?? '—' },
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
}

export function StepDeploy({ values, pending }: StepDeployProps) {
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 p-4">
      <p className="text-sm text-foreground">
        {pending
          ? `Deploying ${values.name}…`
          : `Ready to deploy ${values.name}. This creates the application and starts the first deployment.`}
      </p>
      <p className="text-xs text-muted-foreground">
        Nothing has been submitted yet. Confirm deploy to send the request.
      </p>
    </div>
  )
}
