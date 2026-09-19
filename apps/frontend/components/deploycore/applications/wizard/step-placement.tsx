'use client'

import { useMemo } from 'react'
import { Controller, type Control, type FieldErrors, type UseFormRegister, type UseFormSetValue, type UseFormWatch } from 'react-hook-form'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { projects, servers } from '@/lib/mock-data'
import type { CreateApplicationValues } from '@/lib/validations/application'

interface StepPlacementProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  setValue: UseFormSetValue<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepPlacement({ register, control, watch, setValue, errors }: StepPlacementProps) {
  const projectId = watch('projectId')
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId),
    [projectId],
  )
  const availableServers = servers.filter((server) => server.status !== 'offline')

  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.name) || undefined}>
        <FieldLabel htmlFor="wizard-name">Application name</FieldLabel>
        <Input
          id="wizard-name"
          placeholder="daya-notifications"
          className="font-mono"
          aria-invalid={Boolean(errors.name)}
          {...register('name')}
        />
        <FieldError>{errors.name?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.projectId) || undefined}>
        <FieldLabel htmlFor="wizard-project">Project</FieldLabel>
        <Controller
          name="projectId"
          control={control}
          render={({ field }) => (
            <Select
              value={field.value || null}
              onValueChange={(value) => {
                field.onChange(value ?? '')
                setValue('environment', '')
              }}
            >
              <SelectTrigger id="wizard-project" className="w-full" aria-invalid={Boolean(errors.projectId)}>
                <SelectValue placeholder="Select a project" />
              </SelectTrigger>
              <SelectContent>
                {projects.map((project) => (
                  <SelectItem key={project.id} value={project.id}>
                    {project.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        <FieldError>{errors.projectId?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.environment) || undefined}>
        <FieldLabel htmlFor="wizard-environment">Environment</FieldLabel>
        <Controller
          name="environment"
          control={control}
          render={({ field }) => (
            <Select
              value={field.value || null}
              onValueChange={(value) => field.onChange(value ?? '')}
              disabled={!selectedProject}
            >
              <SelectTrigger
                id="wizard-environment"
                className="w-full"
                aria-invalid={Boolean(errors.environment)}
              >
                <SelectValue placeholder={selectedProject ? 'Select an environment' : 'Select a project first'} />
              </SelectTrigger>
              <SelectContent>
                {(selectedProject?.environments ?? []).map((environment) => (
                  <SelectItem key={environment} value={environment}>
                    {environment}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        <FieldError>{errors.environment?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.serverId) || undefined}>
        <FieldLabel htmlFor="wizard-server">Target server</FieldLabel>
        <Controller
          name="serverId"
          control={control}
          render={({ field }) => (
            <Select value={field.value || null} onValueChange={(value) => field.onChange(value ?? '')}>
              <SelectTrigger id="wizard-server" className="w-full" aria-invalid={Boolean(errors.serverId)}>
                <SelectValue placeholder="Select a server" />
              </SelectTrigger>
              <SelectContent>
                {availableServers.map((server) => (
                  <SelectItem key={server.id} value={server.id}>
                    {server.name} · {server.region}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        <FieldError>{errors.serverId?.message}</FieldError>
      </Field>
    </FieldGroup>
  )
}
