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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  environmentFormSchema,
  slugify,
  type EnvironmentFormValues,
} from '@/lib/validations/project'
import { apiClient, ApiError } from '@/lib/api'

interface EnvironmentFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectName: string
  projectId?: string
  existingNames?: string[]
  onSuccess?: (values: EnvironmentFormValues) => void
}

function EnvironmentFormFields({
  projectName,
  projectId,
  existingNames = [],
  onOpenChange,
  onSuccess,
}: Omit<EnvironmentFormDialogProps, 'open'>) {
  const [serverError, setServerError] = useState<string | null>(null)
  const [slugTouched, setSlugTouched] = useState(false)

  const {
    register,
    control,
    handleSubmit,
    setValue,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<EnvironmentFormValues>({
    resolver: zodResolver(environmentFormSchema),
    defaultValues: {
      name: '',
      slug: '',
      type: 'Staging',
    },
  })

  async function onSubmit(values: EnvironmentFormValues) {
    setServerError(null)
    if (existingNames.some((name) => name.toLowerCase() === values.name.toLowerCase())) {
      setError('name', { message: 'An environment with this name already exists' })
      return
    }

    try {
      if (projectId) {
        try {
          await apiClient.post(`/projects/${projectId}/environments`, {
            name: values.name,
            slug: values.slug,
            kind: values.type.toLowerCase(),
          })
        } catch (err) {
          if (err instanceof ApiError) {
            if (err.status === 409 || err.code === 'CONFLICT') {
              setError('slug', {
                message: 'An environment with this slug already exists in this project',
              })
              return
            }
            setServerError(err.message)
            return
          }
          throw err
        }
      }
      toast.success(`Environment “${values.name}” added to ${projectName}`)
      onSuccess?.(values)
      onOpenChange(false)
    } catch {
      setServerError('Unable to create environment. Please try again.')
    }
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)} noValidate>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="environment-name">Name</FieldLabel>
          <Input
            id="environment-name"
            placeholder="Staging"
            autoComplete="off"
            aria-invalid={Boolean(errors.name)}
            disabled={isSubmitting}
            {...register('name', {
              onChange: (event) => {
                if (!slugTouched) {
                  setValue('slug', slugify(event.target.value), { shouldValidate: false })
                }
              },
            })}
          />
          <FieldError>{errors.name?.message}</FieldError>
        </Field>

        <Field data-invalid={Boolean(errors.slug) || undefined}>
          <FieldLabel htmlFor="environment-slug">Slug</FieldLabel>
          <Controller
            name="slug"
            control={control}
            render={({ field }) => (
              <Input
                id="environment-slug"
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

        <Field data-invalid={Boolean(errors.type) || undefined}>
          <FieldLabel htmlFor="environment-type">Type</FieldLabel>
          <Controller
            name="type"
            control={control}
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value) => {
                  if (value) field.onChange(value)
                }}
                disabled={isSubmitting}
              >
                <SelectTrigger id="environment-type" className="w-full" aria-invalid={Boolean(errors.type)}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="Production">Production</SelectItem>
                  <SelectItem value="Staging">Staging</SelectItem>
                  <SelectItem value="Preview">Preview</SelectItem>
                  <SelectItem value="Development">Development</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
          <FieldError>{errors.type?.message}</FieldError>
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
          {isSubmitting ? 'Creating…' : 'Create environment'}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function EnvironmentFormDialog({
  open,
  onOpenChange,
  projectName,
  projectId,
  existingNames,
  onSuccess,
}: EnvironmentFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New environment</DialogTitle>
          <DialogDescription>
            Add an environment to {projectName} for isolated application deployments.
          </DialogDescription>
        </DialogHeader>
        {open && (
          <EnvironmentFormFields
            key={projectName}
            projectName={projectName}
            projectId={projectId}
            existingNames={existingNames}
            onOpenChange={onOpenChange}
            onSuccess={onSuccess}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
