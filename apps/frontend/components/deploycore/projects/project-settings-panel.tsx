'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import { ProjectFormDialog } from './project-form-dialog'

interface ProjectSettingsPanelProps {
  id?: string
  name: string
  slug: string
  description?: string
  onUpdated?: (updated: { name: string; slug: string; description?: string }) => void
}

export function ProjectSettingsPanel({
  id,
  name,
  slug,
  description,
  onUpdated,
}: ProjectSettingsPanelProps) {
  const router = useRouter()
  const [editOpen, setEditOpen] = useState(false)

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>Project identity used across deployments and routing.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-medium">{name}</p>
            <p className="font-mono text-xs text-muted-foreground">{slug}</p>
            {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
          </div>
          <Button type="button" size="sm" variant="outline" onClick={() => setEditOpen(true)}>
            <Pencil data-icon="inline-start" />
            Edit project
          </Button>
        </CardContent>
      </Card>

      <Card size="sm" className="border-critical/30">
        <CardHeader>
          <CardTitle className="text-critical">Danger zone</CardTitle>
          <CardDescription>
            Deleting a project removes applications, deployments, secrets, and environment config.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DestructiveConfirmDialog
            trigger={
              <Button variant="destructive" size="sm">
                <Trash2 data-icon="inline-start" />
                Delete project
              </Button>
            }
            title={`Delete ${name}?`}
            description="This action is permanent and cannot be undone. Type the project slug to confirm."
            confirmLabel="Delete project"
            confirmationPhrase={slug}
            onConfirm={async () => {
              try {
                const targetId = id || slug
                await apiClient.delete(`/projects/${targetId}`)
                toast.success(`Project “${name}” deleted`)
                router.push('/projects')
              } catch (err) {
                if (err instanceof ApiError) {
                  toast.error(err.message)
                  return
                }
                toast.error(`Unable to delete project “${name}”`)
              }
            }}
          />
        </CardContent>
      </Card>

      <ProjectFormDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        mode="edit"
        project={{ id, name, slug, description }}
        onSuccess={(values) => {
          onUpdated?.(values)
          if (values.slug !== slug) {
            router.push(`/projects/${values.slug}`)
          }
        }}
      />
    </div>
  )
}

interface ProjectsPageActionsProps {
  onCreated?: () => void
}

export function NewProjectButton({ onCreated }: ProjectsPageActionsProps) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        <Plus data-icon="inline-start" />
        New Project
      </Button>
      <ProjectFormDialog
        open={open}
        onOpenChange={setOpen}
        mode="create"
        onSuccess={() => onCreated?.()}
      />
    </>
  )
}
