'use client'

import { useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Database, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  environmentOptionLabel,
  selectItems,
  serverOptionLabel,
  type PlacementEnvironmentOption,
  type PlacementServerOption,
} from '@/components/deploycore/applications/wizard/placement-options'
import { apiClient, ApiError, type Page, type Server, type WireEnvironment, type WireProject } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { createProductionDatabase, projectEnvironmentsPath, projectListPath, serverListPath } from '@/lib/control-plane/database-read'
import {
  DEFAULT_PROVISION_DATABASE_VALUES,
  POSTGRES_VERSIONS,
  provisionDatabaseSchema,
  type ProvisionDatabaseFormValues,
} from '@/lib/validations/database'

interface CreateDatabaseDialogProps {
  onCreated?: () => void
}

export function CreateDatabaseDialog({ onCreated }: CreateDatabaseDialogProps) {
  const router = useRouter()
  const { activeOrg } = useOrganization()
  const [open, setOpen] = useState(false)
  const [projects, setProjects] = useState<WireProject[]>([])
  const [environments, setEnvironments] = useState<PlacementEnvironmentOption[]>([])
  const [servers, setServers] = useState<PlacementServerOption[]>([])
  const [loadingPlacement, setLoadingPlacement] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)
  const [projectId, setProjectId] = useState('')

  const form = useForm<ProvisionDatabaseFormValues>({
    resolver: zodResolver(provisionDatabaseSchema),
    defaultValues: DEFAULT_PROVISION_DATABASE_VALUES,
  })
  const {
    register,
    handleSubmit,
    control,
    setValue,
    reset,
    formState: { errors, isSubmitting },
  } = form

  function loadPlacement(organizationId: string) {
    setLoadingPlacement(true)
    void Promise.all([
      apiClient.get<Page<WireProject>>(projectListPath(organizationId)).catch(() => null),
      apiClient.get<Page<Server>>(serverListPath(organizationId)).catch(() => null),
    ]).then(([projectPage, serverPage]) => {
      setProjects(Array.isArray(projectPage?.items) ? projectPage.items : [])
      setServers(
        (serverPage?.items ?? [])
          .filter((server) => server.id && server.status !== 'OFFLINE')
          .map((server) => ({
            id: server.id || '',
            name: server.name || server.id || '',
            region: server.region || server.provider,
            status: server.status,
          })),
      )
      setLoadingPlacement(false)
    })
  }

  function loadEnvironments(nextProjectId: string) {
    void apiClient
      .get<Page<WireEnvironment>>(projectEnvironmentsPath(nextProjectId))
      .then((page) => {
        setEnvironments(
          (page.items ?? [])
            .filter((environment) => environment.id)
            .map((environment) => ({
              id: environment.id || '',
              name: environment.name || environment.id || '',
              kind: environment.kind,
            })),
        )
      })
      .catch(() => {
        setEnvironments([])
      })
  }

  const projectItems = useMemo(
    () => selectItems(projects.flatMap((project) => (project.id ? [{ id: project.id, name: project.name || project.id }] : [])), (project) => project.name),
    [projects],
  )
  const environmentItems = useMemo(
    () => selectItems(environments, environmentOptionLabel),
    [environments],
  )
  const serverItems = useMemo(() => selectItems(servers, serverOptionLabel), [servers])

  async function onSubmit(data: ProvisionDatabaseFormValues) {
    if (!activeOrg?.id) {
      setServerError('Select an organization before provisioning a database.')
      return
    }
    setServerError(null)
    try {
      const created = await createProductionDatabase(apiClient, {
        organizationId: activeOrg.id,
        projectId: data.projectId,
        environmentId: data.environmentId,
        serverId: data.serverId,
        name: data.name,
        engineVersion: data.engineVersion,
        databaseName: data.databaseName,
        username: data.username,
        storageVolume: data.storageVolume,
      })
      if (!created.id) {
        setServerError('The control plane did not return a database id.')
        return
      }
      toast.success(`Database “${data.name}” is provisioning`)
      onCreated?.()
      setOpen(false)
      reset(DEFAULT_PROVISION_DATABASE_VALUES)
      setProjectId('')
      setEnvironments([])
      router.push(`/databases/${created.id}`)
    } catch (err) {
      const message = err instanceof ApiError || err instanceof Error ? err.message : 'Failed to provision database'
      setServerError(message)
      toast.error(message)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          reset(DEFAULT_PROVISION_DATABASE_VALUES)
          setServerError(null)
          setEnvironments([])
          setProjectId('')
          return
        }
        if (activeOrg?.id) loadPlacement(activeOrg.id)
      }}
    >
      <DialogTrigger
        render={
          <Button size="sm">
            <Plus data-icon="inline-start" />
            Provision database
          </Button>
        }
      />
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Provision managed database</DialogTitle>
          <DialogDescription>
            Creates a PostgreSQL database on the selected environment and server. The control plane generates the password. It is not shown here.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="db-name">Instance name</FieldLabel>
              <Input id="db-name" placeholder="modulyn" aria-invalid={Boolean(errors.name)} {...register('name')} />
              {errors.name ? <FieldError>{errors.name.message}</FieldError> : null}
            </Field>

            <Field>
              <FieldLabel htmlFor="db-version">PostgreSQL version</FieldLabel>
              <Controller
                name="engineVersion"
                control={control}
                render={({ field }) => (
                  <Select
                    items={Object.fromEntries(POSTGRES_VERSIONS.map((version) => [version, `PostgreSQL ${version}`]))}
                    value={field.value || null}
                    onValueChange={(value) => field.onChange(value ?? '')}
                  >
                    <SelectTrigger id="db-version" className="w-full">
                      <SelectValue placeholder="Version" />
                    </SelectTrigger>
                    <SelectContent>
                      {POSTGRES_VERSIONS.map((version) => (
                        <SelectItem key={version} value={version} label={`PostgreSQL ${version}`}>
                          PostgreSQL {version}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </Field>

            <Field data-invalid={Boolean(errors.projectId)}>
              <FieldLabel htmlFor="db-project">Project</FieldLabel>
              <Controller
                name="projectId"
                control={control}
                render={({ field }) => (
                  <Select
                    items={projectItems}
                    value={field.value || null}
                    onValueChange={(value) => {
                      const next = value ?? ''
                      field.onChange(next)
                      setProjectId(next)
                      setValue('environmentId', '')
                      setEnvironments([])
                      if (next) loadEnvironments(next)
                    }}
                  >
                    <SelectTrigger id="db-project" className="w-full">
                      <SelectValue placeholder={loadingPlacement ? 'Loading projects…' : 'Select a project'} />
                    </SelectTrigger>
                    <SelectContent>
                      {projects.map((project) =>
                        project.id ? (
                          <SelectItem key={project.id} value={project.id} label={project.name || project.id}>
                            {project.name || project.id}
                          </SelectItem>
                        ) : null,
                      )}
                    </SelectContent>
                  </Select>
                )}
              />
              {errors.projectId ? <FieldError>{errors.projectId.message}</FieldError> : null}
            </Field>

            <Field data-invalid={Boolean(errors.environmentId)}>
              <FieldLabel htmlFor="db-environment">Environment</FieldLabel>
              <Controller
                name="environmentId"
                control={control}
                render={({ field }) => (
                  <Select
                    items={environmentItems}
                    value={field.value || null}
                    onValueChange={(value) => field.onChange(value ?? '')}
                  >
                    <SelectTrigger id="db-environment" className="w-full">
                      <SelectValue placeholder={projectId ? 'Select an environment' : 'Select a project first'} />
                    </SelectTrigger>
                    <SelectContent>
                      {environments.map((environment) => (
                        <SelectItem key={environment.id} value={environment.id} label={environmentOptionLabel(environment)}>
                          {environmentOptionLabel(environment)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {errors.environmentId ? <FieldError>{errors.environmentId.message}</FieldError> : null}
            </Field>

            <Field data-invalid={Boolean(errors.serverId)}>
              <FieldLabel htmlFor="db-server">Server</FieldLabel>
              <Controller
                name="serverId"
                control={control}
                render={({ field }) => (
                  <Select
                    items={serverItems}
                    value={field.value || null}
                    onValueChange={(value) => field.onChange(value ?? '')}
                  >
                    <SelectTrigger id="db-server" className="w-full">
                      <SelectValue placeholder={loadingPlacement ? 'Loading servers…' : 'Select a server'} />
                    </SelectTrigger>
                    <SelectContent>
                      {servers.map((server) => (
                        <SelectItem key={server.id} value={server.id} label={serverOptionLabel(server)}>
                          {serverOptionLabel(server)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {errors.serverId ? <FieldError>{errors.serverId.message}</FieldError> : null}
            </Field>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field data-invalid={Boolean(errors.databaseName)}>
                <FieldLabel htmlFor="db-logical-name">Database name</FieldLabel>
                <Input id="db-logical-name" placeholder="app" aria-invalid={Boolean(errors.databaseName)} {...register('databaseName')} />
                {errors.databaseName ? <FieldError>{errors.databaseName.message}</FieldError> : null}
              </Field>
              <Field data-invalid={Boolean(errors.username)}>
                <FieldLabel htmlFor="db-username">Username</FieldLabel>
                <Input id="db-username" placeholder="deploycore" aria-invalid={Boolean(errors.username)} {...register('username')} />
                {errors.username ? <FieldError>{errors.username.message}</FieldError> : null}
              </Field>
            </div>

            <Field>
              <FieldLabel htmlFor="db-volume">Storage volume name</FieldLabel>
              <Input id="db-volume" placeholder="Leave empty to let the control plane name it" {...register('storageVolume')} />
              <FieldDescription>Optional. Empty uses the control plane default volume name.</FieldDescription>
            </Field>
          </FieldGroup>

          {serverError ? (
            <p className="text-sm text-critical" role="alert">
              {serverError}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={isSubmitting}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              <Database data-icon="inline-start" />
              {isSubmitting ? 'Provisioning…' : 'Provision database'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
