'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { Braces, FileQuestion, GitBranch, KeyRound, MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { ApplicationLogsPanel } from '@/components/deploycore/applications/application-logs-panel'
import { ApplicationSettingsPanel } from '@/components/deploycore/applications/application-settings-panel'
import { ApplicationSourceEditor } from '@/components/deploycore/applications/application-source-editor'
import { useProductionApplication } from '@/components/deploycore/applications/production-application-shell'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { EmptyState } from '@/components/platform/empty-state'
import { ErrorState } from '@/components/platform/error-state'
import { LoadingState } from '@/components/platform/loading-state'
import { MaskedSecretInput } from '@/components/platform/secret-field'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { apiClient, ApiError } from '@/lib/api'
import {
  createApplicationSecret,
  loadApplicationSecrets,
  type SecretMetadata,
} from '@/lib/applications/application-bootstrap'
import {
  applicationVariableFieldErrors,
  createApplicationVariable,
  deleteApplicationVariable,
  updateApplicationVariable,
  variableErrorMessage,
} from '@/lib/applications/application-variables'
import { useOrganization } from '@/lib/auth-context'
import {
  loadProductionApplicationDeployments,
  loadProductionApplicationRevisions,
  loadProductionVariables,
  toSettingsApplication,
  type DeploymentDetail,
  type RevisionSummary,
  type VariableDetail,
} from '@/lib/control-plane/detail-read'

function show(value: string | null | undefined): string {
  return value && value.trim() ? value : '—'
}

function UnavailableSection({ title, description }: { title: string; description: string }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <EmptyState icon={FileQuestion} title="Nothing to show" description={description} className="border-0" />
      </CardContent>
    </Card>
  )
}

export function ProductionApplicationSection({ section }: { section: string }) {
  const application = useProductionApplication()
  if (!application) return null

  if (section === 'deployments') return <DeploymentsSection applicationId={application.id} />
  if (section === 'environment') return <VariablesSection applicationId={application.id} />
  if (section === 'logs') {
    return (
      <ApplicationLogsPanel
        application={{
          id: application.id,
          name: application.name,
          environment: application.environmentName ?? application.environmentId ?? '',
          status: application.status,
        }}
      />
    )
  }
  if (section === 'settings') {
    return <ProductionApplicationSettings />
  }
  if (section === 'networking') {
    return (
      <UnavailableSection
        title="Networking"
        description="The control plane does not expose an application network list. No network attachments are invented here."
      />
    )
  }
  if (section === 'revisions') return <RevisionsSection applicationId={application.id} />
  if (section === 'secrets') return <SecretsSection applicationId={application.id} />
  if (section === 'domains' || section === 'metrics') {
    return (
      <UnavailableSection
        title={section[0].toUpperCase() + section.slice(1)}
        description="The control plane does not expose this list for the application. Nothing is invented here."
      />
    )
  }
  return (
    <UnavailableSection
      title="Unknown section"
      description="This application section is not available."
    />
  )
}

function ProductionApplicationSettings() {
  const application = useProductionApplication()
  if (!application) return null
  return (
    <div className="flex flex-col gap-4">
      <ApplicationSourceEditor key={application.id} application={application} />
      <ApplicationSettingsPanel application={toSettingsApplication(application)} sourceRows={[]} />
    </div>
  )
}

function DeploymentsSection({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<DeploymentDetail[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadProductionApplicationDeployments(apiClient, activeOrg.id, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setRows(result.value)
        return
      }
      setError(result.kind === 'error' ? result.message : 'Deployments were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId, attempt])

  if (!activeOrg?.id) {
    return <ErrorState title="Could not load deployments" message="Select an organization to load deployments." />
  }
  if (error) {
    return (
      <ErrorState
        title="Could not load deployments"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((value) => value + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading deployments…" />
  if (rows.length === 0) {
    return <EmptyState icon={GitBranch} title="No deployments" description="This application has no deployments in the control plane." />
  }

  return (
    <Card size="sm">
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Deployment</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Trigger</TableHead>
              <TableHead>Error</TableHead>
              <TableHead>Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell>
                  <Link href={`/deployments/${row.id}`} className="font-mono text-xs hover:underline">
                    {row.id}
                  </Link>
                </TableCell>
                <TableCell>{row.status}</TableCell>
                <TableCell>{show(row.trigger)}</TableCell>
                <TableCell className="max-w-64 truncate">{show(row.errorCode ?? row.errorMessage)}</TableCell>
                <TableCell>{show(row.startedAt ?? row.createdAt)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function RevisionsSection({ applicationId }: { applicationId: string }) {
  const [rows, setRows] = useState<RevisionSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    void loadProductionApplicationRevisions(apiClient, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setRows(result.value)
        return
      }
      setError(result.kind === 'error' ? result.message : 'Revisions were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [applicationId, attempt])

  if (error) {
    return (
      <ErrorState
        title="Could not load revisions"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((value) => value + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading revisions…" />
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={GitBranch}
        title="No revisions"
        description="This application has no revisions. Deploy it to create the first revision."
      />
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Revisions</CardTitle>
        <CardDescription>Revision metadata from the control plane. Snapshots are not shown here.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Revision</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell className="font-mono text-xs">
                  {row.revisionNumber == null ? row.id : `r${row.revisionNumber}`}
                </TableCell>
                <TableCell>{row.status || '—'}</TableCell>
                <TableCell>{show(row.createdAt)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function SecretsSection({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<SecretMetadata[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadApplicationSecrets(apiClient, activeOrg.id, applicationId)
      .then((items) => {
        if (!cancelled) setRows(items)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Secrets could not be loaded.')
      })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId, attempt])

  function closeForm() {
    setOpen(false)
    setName('')
    setValue('')
    setFormError(null)
  }

  async function submitSecret() {
    if (!activeOrg?.id || savingRef.current) return
    const trimmed = name.trim()
    if (!trimmed || !value) {
      setFormError('Name and value are required.')
      return
    }
    savingRef.current = true
    setSaving(true)
    setFormError(null)
    try {
      await createApplicationSecret(apiClient, {
        organizationId: activeOrg.id,
        applicationId,
        name: trimmed,
        value,
      })
      setValue('')
      setName('')
      setOpen(false)
      toast.success(`Secret “${trimmed}” created`)
      setRows(null)
      setAttempt((current) => current + 1)
    } catch (err) {
      const message = err instanceof ApiError || err instanceof Error ? err.message : 'Secret could not be created.'
      setFormError(message)
    } finally {
      savingRef.current = false
      setSaving(false)
      setValue('')
    }
  }

  if (!activeOrg?.id) {
    return <ErrorState title="Could not load secrets" message="Select an organization to load secrets." />
  }
  if (error) {
    return (
      <ErrorState
        title="Could not load secrets"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((current) => current + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading secrets…" />

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-end">
        <Button size="sm" type="button" onClick={() => setOpen(true)}>
          New secret
        </Button>
      </div>
      {rows.length === 0 ? (
        <EmptyState
          icon={KeyRound}
          title="No application secrets"
          description="Create an application-scoped secret here. The value is sent once and is not shown again."
        />
      ) : (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Secrets</CardTitle>
            <CardDescription>Metadata for secrets scoped to this application. Values are not displayed.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.id || row.name}>
                    <TableCell className="font-mono text-xs">{row.name}</TableCell>
                    <TableCell>{row.scope}</TableCell>
                    <TableCell>{row.version ?? '—'}</TableCell>
                    <TableCell>{show(row.updatedAt)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!next) closeForm()
          else setOpen(true)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New secret</DialogTitle>
            <DialogDescription>
              Stored for this application only. The value is submitted once and cleared from this form.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="secret-name">Name</FieldLabel>
              <Input
                id="secret-name"
                value={name}
                autoComplete="off"
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="secret-value">Value</FieldLabel>
              <MaskedSecretInput id="secret-value" value={value} onChange={setValue} />
            </Field>
            {formError ? <FieldError>{formError}</FieldError> : null}
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeForm} disabled={saving}>
              Cancel
            </Button>
            <Button type="button" onClick={() => void submitSecret()} disabled={saving}>
              {saving ? 'Creating…' : 'Create secret'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function VariablesSection({ applicationId }: { applicationId: string }) {
  const { activeOrg } = useOrganization()
  const [rows, setRows] = useState<VariableDetail[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<VariableDetail | null>(null)
  const [keyName, setKeyName] = useState('')
  const [value, setValue] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ key?: string; value?: string }>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<VariableDetail | null>(null)
  const savingRef = useRef(false)

  useEffect(() => {
    if (!activeOrg?.id) return
    let cancelled = false
    void loadProductionVariables(apiClient, activeOrg.id, applicationId).then((result) => {
      if (cancelled) return
      if (result.kind === 'ok') {
        setRows(result.value)
        return
      }
      setError(result.kind === 'error' ? result.message : 'Variables were not found.')
    })
    return () => {
      cancelled = true
    }
  }, [activeOrg?.id, applicationId, attempt])

  function closeEditor() {
    setEditorOpen(false)
    setEditing(null)
    setKeyName('')
    setValue('')
    setFieldErrors({})
    setFormError(null)
  }

  function openCreate() {
    setEditing(null)
    setKeyName('')
    setValue('')
    setFieldErrors({})
    setFormError(null)
    setEditorOpen(true)
  }

  function openEdit(row: VariableDetail) {
    setEditing(row)
    setKeyName(row.key)
    setValue(row.value)
    setFieldErrors({})
    setFormError(null)
    setEditorOpen(true)
  }

  function reloadRows() {
    setRows(null)
    setError(null)
    setAttempt((current) => current + 1)
  }

  async function saveVariable() {
    if (!activeOrg?.id || savingRef.current) return
    const errors = applicationVariableFieldErrors(keyName, value)
    if (errors.key || errors.value) {
      setFieldErrors(errors)
      setFormError(null)
      return
    }
    savingRef.current = true
    setSaving(true)
    setFormError(null)
    const savedKey = keyName.trim()
    try {
      if (editing) {
        await updateApplicationVariable(apiClient, editing.id, { key: keyName, value })
      } else {
        await createApplicationVariable(apiClient, {
          organizationId: activeOrg.id,
          applicationId,
          key: keyName,
          value,
        })
      }
      closeEditor()
      toast.success(editing ? `Updated ${savedKey}` : `Added ${savedKey}`)
      reloadRows()
    } catch (err) {
      setFormError(variableErrorMessage(err, [value]))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  async function removeVariable() {
    if (!removeTarget || savingRef.current) return
    const target = removeTarget
    savingRef.current = true
    setSaving(true)
    try {
      await deleteApplicationVariable(apiClient, target.id)
      toast.success(`Removed ${target.key}`)
      reloadRows()
    } catch (err) {
      toast.error(variableErrorMessage(err, [target.value], 'The variable could not be removed.'))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  if (!activeOrg?.id) {
    return <ErrorState title="Could not load variables" message="Select an organization to load variables." />
  }
  if (error) {
    return (
      <ErrorState
        title="Could not load variables"
        message={error}
        onRetry={() => {
          setError(null)
          setRows(null)
          setAttempt((current) => current + 1)
        }}
      />
    )
  }
  if (!rows) return <LoadingState label="Loading variables…" />

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-end">
        <Button size="sm" type="button" onClick={openCreate} disabled={saving}>
          <Plus data-icon="inline-start" />
          Add variable
        </Button>
      </div>
      {rows.length === 0 ? (
        <EmptyState
          icon={Braces}
          title="No variables"
          description="This application has no variables in the control plane."
        />
      ) : (
        <Card size="sm">
          <CardHeader>
            <CardTitle>Environment variables</CardTitle>
            <CardDescription>Values stored for this application. Demo fixtures are not shown.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Key</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead className="w-10 text-right">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.id || row.key}>
                    <TableCell className="font-mono text-xs">{row.key}</TableCell>
                    <TableCell className="font-mono text-xs">{row.value}</TableCell>
                    <TableCell>{row.scope}</TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={`Actions for ${row.key}`}
                              disabled={!row.id || saving}
                            />
                          }
                        >
                          <MoreHorizontal />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(row)} disabled={!row.id}>
                            <Pencil />
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => setRemoveTarget(row)} disabled={!row.id}>
                            <Trash2 />
                            Remove
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      <Dialog
        open={editorOpen}
        onOpenChange={(next) => {
          if (!next) closeEditor()
          else setEditorOpen(true)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? `Edit ${editing.key}` : 'Add variable'}</DialogTitle>
            <DialogDescription>Stored for this application.</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={Boolean(fieldErrors.key)}>
              <FieldLabel htmlFor="variable-key">Key</FieldLabel>
              <Input
                id="variable-key"
                className="font-mono"
                value={keyName}
                autoComplete="off"
                aria-invalid={Boolean(fieldErrors.key)}
                onChange={(event) => {
                  setKeyName(event.target.value)
                  if (fieldErrors.key) setFieldErrors((current) => ({ ...current, key: undefined }))
                }}
                placeholder="DATABASE_URL"
              />
              {fieldErrors.key ? <FieldError>{fieldErrors.key}</FieldError> : null}
            </Field>
            <Field data-invalid={Boolean(fieldErrors.value)}>
              <FieldLabel htmlFor="variable-value">Value</FieldLabel>
              <Input
                id="variable-value"
                className="font-mono"
                value={value}
                autoComplete="off"
                aria-invalid={Boolean(fieldErrors.value)}
                onChange={(event) => {
                  setValue(event.target.value)
                  if (fieldErrors.value) setFieldErrors((current) => ({ ...current, value: undefined }))
                }}
              />
              {fieldErrors.value ? <FieldError>{fieldErrors.value}</FieldError> : null}
            </Field>
            {formError ? <FieldError>{formError}</FieldError> : null}
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeEditor} disabled={saving}>
              Cancel
            </Button>
            <Button type="button" onClick={() => void saveVariable()} disabled={saving}>
              {saving ? 'Saving…' : 'Save variable'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {removeTarget ? (
        <DestructiveConfirmDialog
          open
          pending={saving}
          onOpenChange={(next) => {
            if (!next) setRemoveTarget(null)
          }}
          title={`Remove ${removeTarget.key}?`}
          description={`This removes ${removeTarget.key} from this application.`}
          confirmLabel="Remove variable"
          confirmationPhrase={removeTarget.key}
          onConfirm={removeVariable}
        />
      ) : null}
    </div>
  )
}
