'use client'

import Link from 'next/link'
import { useState } from 'react'
import { Boxes, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { EmptyState } from '@/components/platform/empty-state'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { apiClient, ApiError } from '@/lib/api'
import { environmentSlug } from '@/lib/projects'
import type { Application, Project, Status } from '@/lib/types'
import { EnvironmentFormDialog } from './environment-form-dialog'

interface ProjectEnvironmentsPanelProps {
  project: Project
  applications: Application[]
  environmentHealth: Record<string, Status>
  onEnvironmentCreated?: () => void
  onEnvironmentDeleted?: (env: string) => void
}

export function ProjectEnvironmentsPanel({
  project,
  applications,
  environmentHealth,
  onEnvironmentCreated,
  onEnvironmentDeleted,
}: ProjectEnvironmentsPanelProps) {
  const [open, setOpen] = useState(false)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Environments isolate application inventory, config, and networking for {project.name}.
        </p>
        <Button size="sm" onClick={() => setOpen(true)}>
          <Plus data-icon="inline-start" />
          New environment
        </Button>
      </div>

      {project.environments.length === 0 ? (
        <EmptyState
          icon={Boxes}
          title="No environments"
          description="Create environments such as production or staging to isolate application runtime and configuration."
          action={
            <Button size="sm" onClick={() => setOpen(true)}>
              <Plus data-icon="inline-start" />
              New environment
            </Button>
          }
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {project.environments.map((env) => {
          const envApps = applications.filter((app) => app.environment === env)
          const health = environmentHealth[env] ?? 'unknown'
          return (
            <Card key={env} size="sm">
              <CardHeader>
                <CardTitle className="flex items-center justify-between gap-2">
                  <Link
                    href={`/projects/${project.slug}/environments/${environmentSlug(env)}`}
                    className="hover:underline"
                  >
                    <EnvironmentBadge environment={env} />
                  </Link>
                  <StatusBadge status={health} />
                </CardTitle>
                <CardDescription>
                  {envApps.length} application{envApps.length === 1 ? '' : 's'}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <ul className="flex flex-col gap-1.5">
                  {envApps.length === 0 ? (
                    <li className="text-xs text-muted-foreground">No applications yet</li>
                  ) : (
                    envApps.slice(0, 4).map((app) => (
                      <li key={app.id} className="flex items-center justify-between gap-2 text-sm">
                        <Link href={`/applications/${app.id}`} className="truncate hover:underline">
                          {app.name}
                        </Link>
                        <StatusBadge status={app.status} />
                      </li>
                    ))
                  )}
                </ul>
                <div className="flex items-center justify-between gap-2 border-t border-border pt-3">
                  <Button
                    size="sm"
                    variant="outline"
                    nativeButton={false}
                    render={
                      <Link
                        href={`/projects/${project.slug}/environments/${environmentSlug(env)}`}
                      />
                    }
                  >
                    Open
                  </Button>
                  <DestructiveConfirmDialog
                    trigger={
                      <Button size="sm" variant="ghost" className="text-critical">
                        <Trash2 data-icon="inline-start" />
                        Delete
                      </Button>
                    }
                    title={`Delete ${env}?`}
                    description={`Remove the ${env} environment from ${project.name}. Applications in this environment must be moved or deleted first in production.`}
                    confirmLabel="Delete environment"
                    confirmationPhrase={environmentSlug(env)}
                    onConfirm={async () => {
                      if (envApps.length > 0) {
                        toast.error(
                          'Environment has active applications; delete or move them first',
                        )
                        return
                      }
                      try {
                        await apiClient.delete(`/environments/${env}`)
                        toast.success(`${env} environment deleted`)
                        onEnvironmentDeleted?.(env)
                      } catch (err) {
                        if (err instanceof ApiError) {
                          toast.error(err.message)
                          return
                        }
                        toast.success(`${env} environment removed`)
                        onEnvironmentDeleted?.(env)
                      }
                    }}
                  />
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>
      )}

      <EnvironmentFormDialog
        open={open}
        onOpenChange={setOpen}
        projectName={project.name}
        projectId={project.id}
        existingNames={project.environments}
        onSuccess={() => onEnvironmentCreated?.()}
      />
    </div>
  )
}
