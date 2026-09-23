'use client'

import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  projectFormSchema,
  slugify,
  type ProjectFormValues,
} from '@/lib/validations/project'
import { apiClient, ApiError } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import type { Project } from '@/lib/types'

interface ProjectFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  project?: Pick<Project, 'name' | 'slug'> & { id?: string; description?: string }
  onSuccess?: (values: ProjectFormValues) => void
}

function ProjectFormFields({
  mode,
  project,
  onOpenChange,
  onSuccess,
}: Omit<ProjectFormDialogProps, 'open'>) {
  const { activeOrg } = useOrganization()
  const [serverError, setServerError] = useState<string | null>(null)
  const [slugTouched, setSlugTouched] = useState(mode === 'edit')

  const {
    register,
    control,
    handleSubmit,
    setValue,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ProjectFormValues>({
    resolver: zodResolver(projectFormSchema),
    defaultValues: {
      name: project?.name ?? '',
      slug: project?.slug ?? '',
      description: project?.description ?? '',
    },
  })

  async function onSubmit(values: ProjectFormValues) {
    setServerError(null)
    try {
      if (mode === 'create') {
        const orgId = activeOrg?.id
        if (orgId) {
          try {
            await apiClient.post('/projects', {
              organizationId: orgId,
              name: values.name,
              slug: values.slug,
              description: values.description ?? '',
            })
          } catch (err) {
            if (err instanceof ApiError) {
              if (err.status === 409 || err.code === 'CONFLICT') {
                setError('slug', {
                  message: 'A project with this slug already exists in this organization',
                })
                return
              }
              setServerError(err.message)
              return
            }
            throw err
          }
        }
        toast.success(`Project “${values.name}” created`)
      } else {
        const targetId = project?.id || project?.slug
        if (targetId) {
          try {
            await apiClient.patch(`/projects/${targetId}`, {
              name: values.name,
              slug: values.slug,
              description: values.description,
            })
          } catch (err) {
            if (err instanceof ApiError) {
              if (err.status === 409 || err.code === 'CONFLICT') {
                setError('slug', {
                  message: 'A project with this slug already exists',
                })
                return
              }
              setServerError(err.message)
              return
            }
            throw err
          }
        }
        toast.success(`Project “${values.name}” updated`)
      }
      onSuccess?.(values)
      onOpenChange(false)
    } catch {
      setServerError('Unable to save project. Please try again.')
    }
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)} noValidate>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="project-name">Name</FieldLabel>
          <Input
            id="project-name"
            autoComplete="off"
            aria-invalid={Boolean(errors.name)}
            disabled={isSubmitting}
            {...register('name', {
              onChange: (event) => {
                if (!slugTouched && mode === 'create') {
                  setValue('slug', slugify(event.target.value), { shouldValidate: false })
                }
              },
            })}
          />
          <FieldError>{errors.name?.message}</FieldError>
        </Field>

        <Field data-invalid={Boolean(errors.slug) || undefined}>
          <FieldLabel htmlFor="project-slug">Slug</FieldLabel>
          <Controller
            name="slug"
            control={control}
            render={({ field }) => (
              <Input
                id="project-slug"
                className="font-mono"
                autoComplete="off"
                aria-invalid={Boolean(errors.slug)}
                disabled={isSubmitting}
                {...field}
                onChange={(event) => {
                  setSlugTouched(true)
                  field.onChange(slugify(event.target.value))
                }}
              />
            )}
          />
          <FieldError>{errors.slug?.message}</FieldError>
        </Field>

        <Field data-invalid={Boolean(errors.description) || undefined}>
          <FieldLabel htmlFor="project-description">Description</FieldLabel>
          <Textarea
            id="project-description"
            rows={3}
            disabled={isSubmitting}
            aria-invalid={Boolean(errors.description)}
            {...register('description')}
          />
          <FieldError>{errors.description?.message}</FieldError>
        </Field>
      </FieldGroup>

      {serverError && (
        <p className="text-sm text-critical" role="alert">
          {serverError}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" disabled={isSubmitting} onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting ? 'Saving…' : mode === 'create' ? 'Create project' : 'Save changes'}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function ProjectFormDialog({
  open,
  onOpenChange,
  mode,
  project,
  onSuccess,
}: ProjectFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{mode === 'create' ? 'New project' : 'Edit project'}</DialogTitle>
          <DialogDescription>
            {mode === 'create'
              ? 'Create a project to group applications, environments, and shared resources.'
              : 'Update project identity. Changing the slug may break existing links.'}
          </DialogDescription>
        </DialogHeader>
        {open && (
          <ProjectFormFields
            key={`${mode}-${project?.slug ?? 'new'}`}
            mode={mode}
            project={project}
            onOpenChange={onOpenChange}
            onSuccess={onSuccess}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
