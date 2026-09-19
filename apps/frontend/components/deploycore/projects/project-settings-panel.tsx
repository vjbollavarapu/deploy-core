'use client'

import { useState } from 'react'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { ProjectFormDialog } from './project-form-dialog'

interface ProjectSettingsPanelProps {
  name: string
  slug: string
}

export function ProjectSettingsPanel({ name, slug }: ProjectSettingsPanelProps) {
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
            onConfirm={() => {
              toast.success(`${name} deleted`)
            }}
          />
        </CardContent>
      </Card>

      <ProjectFormDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        mode="edit"
        project={{ name, slug }}
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
