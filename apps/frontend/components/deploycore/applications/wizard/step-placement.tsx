'use client'

import { useMemo } from 'react'
import {
  Controller,
  type Control,
  type FieldErrors,
  type UseFormRegister,
  type UseFormSetValue,
  type UseFormWatch,
} from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { CreateApplicationValues } from '@/lib/validations/application'
import {
  environmentOptionLabel,
  environmentSelectItems,
  placementAfterProjectChange,
  projectSelectItems,
  serverOptionLabel,
  serverSelectItems,
} from './placement-options'

export interface PlacementEnvironment {
  id: string
  name: string
  kind?: string
}

export interface PlacementProject {
  id: string
  name: string
  slug?: string
  environments: PlacementEnvironment[]
}

export interface PlacementServer {
  id: string
  name: string
  region?: string
  status?: string
}

interface StepPlacementProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  setValue: UseFormSetValue<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  projects: PlacementProject[]
  servers: PlacementServer[]
  isLoading?: boolean
}

export function StepPlacement({
  register,
  control,
  watch,
  setValue,
  errors,
  projects,
  servers,
  isLoading = false,
}: StepPlacementProps) {
  const projectId = watch('projectId')
  const serverId = watch('serverId')
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === projectId),
    [projects, projectId],
  )
  const projectItems = useMemo(() => projectSelectItems(projects), [projects])
  const environmentItems = useMemo(
    () => environmentSelectItems(projects, projectId),
    [projects, projectId],
  )
  const availableServers = useMemo(
    () => servers.filter((server) => server.status !== 'offline' && server.status !== 'OFFLINE'),
    [servers],
  )
  const serverItems = useMemo(() => serverSelectItems(servers), [servers])

  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.name) || undefined}>
        <FieldLabel htmlFor="wizard-name">Application name</FieldLabel>
        <Input
          id="wizard-name"
          placeholder="order-service"
          className="font-mono"
          aria-invalid={Boolean(errors.name)}
          {...register('name')}
        />
        <FieldDescription>Lowercase letters, numbers, and hyphens (DNS compatible).</FieldDescription>
        <FieldError>{errors.name?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.projectId) || undefined}>
        <FieldLabel htmlFor="wizard-project">Project</FieldLabel>
        <Controller
          name="projectId"
          control={control}
          render={({ field }) => (
            <Select
              items={projectItems}
              value={field.value || null}
              onValueChange={(value) => {
                const next = placementAfterProjectChange(serverId)
                field.onChange(value ?? '')
                setValue('environment', next.environment)
                setValue('serverId', next.serverId)
              }}
            >
              <SelectTrigger id="wizard-project" className="w-full" aria-invalid={Boolean(errors.projectId)}>
                <SelectValue placeholder={isLoading ? 'Loading projects…' : 'Select a project'} />
              </SelectTrigger>
              <SelectContent>
                {projects.map((project) => (
                  <SelectItem key={project.id} value={project.id} label={project.name}>
                    {projectItems[project.id]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {projects.length === 0 && !isLoading ? (
          <FieldDescription className="text-warning">No projects found. Create a project first.</FieldDescription>
        ) : null}
        <FieldError>{errors.projectId?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.environment) || undefined}>
        <FieldLabel htmlFor="wizard-environment">Environment</FieldLabel>
        <Controller
          name="environment"
          control={control}
          render={({ field }) => (
            <Select
              items={environmentItems}
              value={field.value || null}
              onValueChange={(value) => field.onChange(value ?? '')}
              disabled={!selectedProject}
            >
              <SelectTrigger
                id="wizard-environment"
                className="w-full"
                aria-invalid={Boolean(errors.environment)}
              >
                <SelectValue
                  placeholder={
                    selectedProject
                      ? selectedProject.environments.length === 0
                        ? 'No environments configured'
                        : 'Select an environment'
                      : 'Select a project first'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(selectedProject?.environments ?? []).map((env) => (
                  <SelectItem key={env.id} value={env.id} label={environmentOptionLabel(env)}>
                    {environmentItems[env.id]}
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
            <Select
              items={serverItems}
              value={field.value || null}
              onValueChange={(value) => field.onChange(value ?? '')}
            >
              <SelectTrigger id="wizard-server" className="w-full" aria-invalid={Boolean(errors.serverId)}>
                <SelectValue placeholder={isLoading ? 'Loading servers…' : 'Select a server'} />
              </SelectTrigger>
              <SelectContent>
                {availableServers.map((server) => (
                  <SelectItem key={server.id} value={server.id} label={serverOptionLabel(server)}>
                    {serverItems[server.id]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        {availableServers.length === 0 && !isLoading ? (
          <FieldDescription className="text-warning">No online servers available in this organization.</FieldDescription>
        ) : null}
        <FieldError>{errors.serverId?.message}</FieldError>
      </Field>
    </FieldGroup>
  )
}
