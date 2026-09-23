'use client'

import { useId, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
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
import { Switch } from '@/components/ui/switch'
import {
  DATABASE_ENGINES,
  POSTGRES_VERSIONS,
  createDatabaseSchema,
  DEFAULT_CREATE_DATABASE_VALUES,
  type CreateDatabaseFormValues,
} from '@/lib/validations/database'
import { projects as rawProjects, servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const projects = getDemoFixtures(rawProjects)
const servers = getDemoFixtures(rawServers)
import type { DatabaseInstance } from '@/lib/types'

interface CreateDatabaseDialogProps {
  onCreated?: (database: DatabaseInstance) => void
}

export function CreateDatabaseDialog({ onCreated }: CreateDatabaseDialogProps) {
  const [open, setOpen] = useState(false)

  const nameInputId = useId()
  const engineSelectId = useId()
  const versionSelectId = useId()
  const projectSelectId = useId()
  const envSelectId = useId()
  const serverSelectId = useId()
  const dbNameInputId = useId()
  const usernameInputId = useId()
  const storageInputId = useId()
  const credPolicySwitchId = useId()

  const {
    register,
    handleSubmit,
    setValue,
    reset,
    control,
    formState: { errors, isSubmitting },
  } = useForm<CreateDatabaseFormValues>({
    resolver: zodResolver(createDatabaseSchema),
    defaultValues: {
      ...DEFAULT_CREATE_DATABASE_VALUES,
      project: projects[0]?.name ?? 'Daya Platform',
    },
  })

  const selectedEngine = useWatch({ control, name: 'type', defaultValue: 'PostgreSQL' })
  const selectedVersion = useWatch({ control, name: 'version', defaultValue: '16' })
  const selectedProject = useWatch({ control, name: 'project', defaultValue: projects[0]?.name ?? '' })
  const selectedEnvironment = useWatch({ control, name: 'environment', defaultValue: 'Production' })
  const selectedServer = useWatch({ control, name: 'server', defaultValue: servers[0]?.name ?? '' })
  const credReveal = useWatch({ control, name: 'credentialsRevealAllowed', defaultValue: true })

  async function onSubmit(data: CreateDatabaseFormValues) {
    try {
      const newInstance: DatabaseInstance = {
        id: `db-${data.name}`,
        name: data.name,
        type: data.type,
        version: data.version,
        project: data.project,
        environment: data.environment,
        server: data.server,
        storageUsedGb: 1,
        storageTotalGb: data.storageTotalGb,
        backups: 0,
        lastBackup: 'Never',
        status: 'healthy',
        dbName: data.dbName,
        port: data.type === 'PostgreSQL' ? 5432 : data.type === 'MySQL' ? 3306 : data.type === 'Redis' ? 6379 : 27017,
        username: data.username,
        connectionHost: `${data.name}.${data.project.toLowerCase().replace(/\s+/g, '-')}.internal`,
        credentialsRevealAllowed: data.credentialsRevealAllowed,
      }

      onCreated?.(newInstance)
      toast.success(`${newInstance.name} provisioned successfully (${newInstance.type} ${newInstance.version})`)
      setOpen(false)
      reset()
    } catch {
      toast.error('Failed to provision database')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
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
            Deploy a containerized, managed database engine with automated storage volumes and backup scheduling. PostgreSQL is the default engine.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor={nameInputId}>Instance name</FieldLabel>
              <Input
                id={nameInputId}
                placeholder="analytics-db"
                aria-invalid={Boolean(errors.name)}
                {...register('name')}
              />
              <FieldDescription>Lowercase alphanumeric identifier with hyphens.</FieldDescription>
              {errors.name ? <FieldError>{errors.name.message}</FieldError> : null}
            </Field>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={engineSelectId}>Engine</FieldLabel>
                <Select
                  value={selectedEngine}
                  onValueChange={(val) => {
                    if (val) setValue('type', val as typeof selectedEngine)
                  }}
                >
                  <SelectTrigger id={engineSelectId} className="w-full">
                    <SelectValue placeholder="Select engine" />
                  </SelectTrigger>
                  <SelectContent>
                    {DATABASE_ENGINES.map((engine) => (
                      <SelectItem key={engine} value={engine}>
                        {engine}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor={versionSelectId}>Engine version</FieldLabel>
                <Select
                  value={selectedVersion}
                  onValueChange={(val) => {
                    if (val) setValue('version', val)
                  }}
                >
                  <SelectTrigger id={versionSelectId} className="w-full">
                    <SelectValue placeholder="Version" />
                  </SelectTrigger>
                  <SelectContent>
                    {POSTGRES_VERSIONS.map((ver) => (
                      <SelectItem key={ver} value={ver}>
                        PostgreSQL {ver}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={projectSelectId}>Project</FieldLabel>
                <Select
                  value={selectedProject}
                  onValueChange={(val) => {
                    if (val) setValue('project', val)
                  }}
                >
                  <SelectTrigger id={projectSelectId} className="w-full">
                    <SelectValue placeholder="Project" />
                  </SelectTrigger>
                  <SelectContent>
                    {projects.map((p) => (
                      <SelectItem key={p.id} value={p.name}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor={envSelectId}>Environment</FieldLabel>
                <Select
                  value={selectedEnvironment}
                  onValueChange={(val) => {
                    if (val) setValue('environment', val)
                  }}
                >
                  <SelectTrigger id={envSelectId} className="w-full">
                    <SelectValue placeholder="Environment" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Production">Production</SelectItem>
                    <SelectItem value="Staging">Staging</SelectItem>
                    <SelectItem value="Development">Development</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <Field>
              <FieldLabel htmlFor={serverSelectId}>Target server placement</FieldLabel>
              <Select
                value={selectedServer}
                onValueChange={(val) => {
                  if (val) setValue('server', val)
                }}
              >
                <SelectTrigger id={serverSelectId} className="w-full">
                  <SelectValue placeholder="Select server" />
                </SelectTrigger>
                <SelectContent>
                  {servers.map((s) => (
                    <SelectItem key={s.id} value={s.name}>
                      {s.name} ({s.region}) · {s.status}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field data-invalid={Boolean(errors.dbName)}>
                <FieldLabel htmlFor={dbNameInputId}>Database name</FieldLabel>
                <Input
                  id={dbNameInputId}
                  placeholder="analytics"
                  aria-invalid={Boolean(errors.dbName)}
                  {...register('dbName')}
                />
                {errors.dbName ? <FieldError>{errors.dbName.message}</FieldError> : null}
              </Field>

              <Field data-invalid={Boolean(errors.username)}>
                <FieldLabel htmlFor={usernameInputId}>Initial username</FieldLabel>
                <Input
                  id={usernameInputId}
                  placeholder="postgres"
                  aria-invalid={Boolean(errors.username)}
                  {...register('username')}
                />
                {errors.username ? <FieldError>{errors.username.message}</FieldError> : null}
              </Field>
            </div>

            <Field data-invalid={Boolean(errors.storageTotalGb)}>
              <FieldLabel htmlFor={storageInputId}>Storage volume (GB)</FieldLabel>
              <Input
                id={storageInputId}
                type="number"
                min={5}
                max={10000}
                aria-invalid={Boolean(errors.storageTotalGb)}
                {...register('storageTotalGb', { valueAsNumber: true })}
              />
              <FieldDescription>Initial persistent disk allocation attached to container.</FieldDescription>
              {errors.storageTotalGb ? <FieldError>{errors.storageTotalGb.message}</FieldError> : null}
            </Field>

            <Field className="flex flex-row items-center justify-between gap-3 rounded-lg border border-border p-3">
              <div className="flex flex-col gap-0.5">
                <FieldLabel htmlFor={credPolicySwitchId} className="cursor-pointer">Credential reveal policy</FieldLabel>
                <FieldDescription>
                  When enabled, operators with proper permissions can reveal the live connection password.
                </FieldDescription>
              </div>
              <Switch
                id={credPolicySwitchId}
                checked={credReveal}
                onCheckedChange={(checked) => setValue('credentialsRevealAllowed', checked)}
              />
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={isSubmitting}
            >
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
