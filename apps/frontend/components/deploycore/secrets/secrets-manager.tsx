'use client'

import { useId, useMemo, useState } from 'react'
import {
  KeyRound,
  Lock,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Search,
  Shield,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { FilterBar } from '@/components/platform/filter-bar'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { EmptyState } from '@/components/platform/empty-state'
import { MaskedSecretInput } from '@/components/platform/secret-field'
import {
  ACCESS_ROLES,
  SECRET_SCOPES,
  addSecretSchema,
  rotateSecretSchema,
  type SecretScope,
} from '@/lib/validations/secret'
import type { SecretItem } from '@/lib/types'

interface SecretsManagerProps {
  initialSecrets: SecretItem[]
}

export function SecretsManager({ initialSecrets }: SecretsManagerProps) {
  const [secrets, setSecrets] = useState<SecretItem[]>(initialSecrets)
  const [query, setQuery] = useState('')
  const [scopeFilter, setScopeFilter] = useState('all')

  const [createOpen, setCreateOpen] = useState(false)
  const [rotateTarget, setRotateTarget] = useState<SecretItem | null>(null)
  const [accessTarget, setAccessTarget] = useState<SecretItem | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<SecretItem | null>(null)

  // Create form state
  const [name, setName] = useState('')
  const [scope, setScope] = useState<SecretScope>('Application')
  const [apps, setApps] = useState('')
  const [description, setDescription] = useState('')
  const [newValue, setNewValue] = useState('')
  const [createErrors, setCreateErrors] = useState<{
    name?: string
    value?: string
    scope?: string
  }>({})

  // Rotate form state
  const [rotateValue, setRotateValue] = useState('')
  const [rotateError, setRotateError] = useState<string | null>(null)

  // Scoped access state
  const [accessRoles, setAccessRoles] = useState<string[]>([])

  const nameInputId = useId()
  const scopeInputId = useId()
  const appsInputId = useId()
  const descInputId = useId()
  const valueInputId = useId()
  const rotateValueId = useId()

  function resetCreate() {
    setName('')
    setScope('Application')
    setApps('')
    setDescription('')
    setNewValue('')
    setCreateErrors({})
  }

  function createSecret() {
    const result = addSecretSchema.safeParse({
      name,
      scope,
      applications: apps,
      description,
      value: newValue,
      accessRoles: ['Owner', 'Admin'],
    })

    if (!result.success) {
      const errMap: { name?: string; value?: string; scope?: string } = {}
      for (const issue of result.error.issues) {
        const path = issue.path[0] as 'name' | 'value' | 'scope'
        if (path && !errMap[path]) {
          errMap[path] = issue.message
        }
      }
      setCreateErrors(errMap)
      return
    }

    const item: SecretItem = {
      id: `sec-${Date.now()}`,
      name: name.trim(),
      scope,
      applications: apps
        .split(',')
        .map((a) => a.trim())
        .filter(Boolean),
      updatedAt: 'just now',
      updatedBy: 'You',
      lastRotatedAt: 'just now',
      accessRoles: ['Owner', 'Admin'],
      description: description.trim() || undefined,
    }

    setSecrets((prev) => [item, ...prev])
    toast.success(`${item.name} encrypted and created`)
    setCreateOpen(false)
    resetCreate()
  }

  function confirmRotate() {
    if (!rotateTarget) return
    const result = rotateSecretSchema.safeParse({ value: rotateValue })
    if (!result.success) {
      setRotateError(result.error.issues[0]?.message ?? 'New secret value is required')
      return
    }

    setSecrets((prev) =>
      prev.map((secret) =>
        secret.id === rotateTarget.id
          ? {
              ...secret,
              updatedAt: 'just now',
              updatedBy: 'You',
              lastRotatedAt: 'just now',
            }
          : secret,
      ),
    )
    toast.success(`${rotateTarget.name} rotated successfully`)
    setRotateTarget(null)
    setRotateValue('')
    setRotateError(null)
  }

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return secrets.filter((s) => {
      if (scopeFilter !== 'all' && s.scope !== scopeFilter) return false
      if (!q) return true
      return (
        s.name.toLowerCase().includes(q) ||
        (s.description && s.description.toLowerCase().includes(q)) ||
        s.applications.some((app) => app.toLowerCase().includes(q))
      )
    })
  }, [secrets, query, scopeFilter])

  const sorted = useMemo(
    () => [...filtered].sort((a, b) => a.name.localeCompare(b.name)),
    [filtered],
  )

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <Lock className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
          <span className="text-xs font-medium text-foreground">AES-256-GCM Envelope Encryption</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Metadata only — secret values are write-only and never listed, logged, or repopulated from the server.
        </p>
      </div>

      <FilterBar
        end={
          <Dialog
            open={createOpen}
            onOpenChange={(open) => {
              setCreateOpen(open)
              if (!open) resetCreate()
            }}
          >
            <DialogTrigger
              render={
                <Button size="sm">
                  <Plus data-icon="inline-start" />
                  New secret
                </Button>
              }
            />
            <DialogContent className="sm:max-w-md">
              <DialogHeader>
                <DialogTitle>New encrypted secret</DialogTitle>
                <DialogDescription>
                  Values are write-only and envelope-encrypted at rest. DeployCore never echoes plaintext to the UI.
                </DialogDescription>
              </DialogHeader>
              <FieldGroup>
                <Field data-invalid={Boolean(createErrors.name)}>
                  <FieldLabel htmlFor={nameInputId}>Name</FieldLabel>
                  <Input
                    id={nameInputId}
                    className="font-mono"
                    value={name}
                    aria-invalid={Boolean(createErrors.name)}
                    onChange={(e) => {
                      setName(e.target.value)
                      if (createErrors.name) setCreateErrors((prev) => ({ ...prev, name: undefined }))
                    }}
                    placeholder="API_SIGNING_KEY"
                  />
                  {createErrors.name ? <FieldError>{createErrors.name}</FieldError> : null}
                </Field>

                <Field>
                  <FieldLabel htmlFor={scopeInputId}>Scope</FieldLabel>
                  <Select
                    value={scope}
                    onValueChange={(v) => setScope((v as SecretScope) ?? 'Application')}
                  >
                    <SelectTrigger id={scopeInputId} className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SECRET_SCOPES.map((s) => (
                        <SelectItem key={s} value={s}>
                          {s}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                <Field>
                  <FieldLabel htmlFor={appsInputId}>Applications</FieldLabel>
                  <Input
                    id={appsInputId}
                    value={apps}
                    onChange={(e) => setApps(e.target.value)}
                    placeholder="daya-api, daya-web"
                  />
                  <FieldDescription>Comma-separated application names with access.</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor={descInputId}>Description</FieldLabel>
                  <Textarea
                    id={descInputId}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    className="min-h-16 text-xs"
                    placeholder="What this secret is used for..."
                  />
                </Field>

                <Field data-invalid={Boolean(createErrors.value)}>
                  <FieldLabel htmlFor={valueInputId}>Secret value</FieldLabel>
                  <MaskedSecretInput
                    id={valueInputId}
                    value={newValue}
                    aria-invalid={Boolean(createErrors.value)}
                    onChange={(val) => {
                      setNewValue(val)
                      if (createErrors.value) setCreateErrors((prev) => ({ ...prev, value: undefined }))
                    }}
                    placeholder="Enter secret value"
                  />
                  <FieldDescription>Encrypted upon creation with AES-256-GCM.</FieldDescription>
                  {createErrors.value ? <FieldError>{createErrors.value}</FieldError> : null}
                </Field>
              </FieldGroup>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                  Cancel
                </Button>
                <Button type="button" onClick={createSecret}>
                  <KeyRound data-icon="inline-start" />
                  Save secret
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        }
      >
        <InputGroup className="max-w-xs">
          <InputGroupInput
            placeholder="Search secrets…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
        </InputGroup>
        <Select value={scopeFilter} onValueChange={(v) => setScopeFilter(v ?? 'all')}>
          <SelectTrigger className="w-44">
            <SelectValue placeholder="Scope" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All scopes</SelectItem>
            {SECRET_SCOPES.map((s) => (
              <SelectItem key={s} value={s}>
                {s}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FilterBar>

      <div className="rounded-lg border border-border overflow-hidden">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Applications</TableHead>
                <TableHead>Access</TableHead>
                <TableHead>Rotated</TableHead>
                <TableHead>Updated</TableHead>
                <TableHead className="w-10 text-right">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((secret) => (
                <TableRow key={secret.id}>
                  <TableCell>
                    <div className="flex flex-col gap-0.5">
                      <span className="font-mono text-xs font-semibold">{secret.name}</span>
                      {secret.description ? (
                        <span className="text-[11px] text-muted-foreground">{secret.description}</span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="text-[10px]">{secret.scope}</Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {secret.applications.length > 0 ? (
                        secret.applications.map((app) => (
                          <Badge key={app} variant="outline" className="font-mono text-[10px]">
                            {app}
                          </Badge>
                        ))
                      ) : (
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {secret.accessRoles.map((role) => (
                        <Badge key={role} variant="outline" className="text-[10px]">
                          {role}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{secret.lastRotatedAt}</TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5 text-xs">
                      <span>{secret.updatedAt}</span>
                      <span className="text-muted-foreground text-[11px]">by {secret.updatedBy}</span>
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`Actions for ${secret.name}`}
                          />
                        }
                      >
                        <MoreHorizontal />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          onClick={() => {
                            setRotateValue('')
                            setRotateError(null)
                            setRotateTarget(secret)
                          }}
                        >
                          <RefreshCw />
                          Rotate secret
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => {
                            setAccessRoles([...secret.accessRoles])
                            setAccessTarget(secret)
                          }}
                        >
                          <Shield />
                          Scoped access
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleteTarget(secret)}
                        >
                          <Trash2 />
                          Delete secret
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
              {sorted.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="py-12">
                    <EmptyState
                      icon={KeyRound}
                      title="No secrets found"
                      description={
                        query || scopeFilter !== 'all'
                          ? 'No secrets match your filter criteria.'
                          : 'No encrypted secrets configured yet.'
                      }
                      action={
                        query || scopeFilter !== 'all' ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setQuery('')
                              setScopeFilter('all')
                            }}
                          >
                            Reset filters
                          </Button>
                        ) : (
                          <Button size="sm" onClick={() => setCreateOpen(true)}>
                            <Plus data-icon="inline-start" />
                            New secret
                          </Button>
                        )
                      }
                      className="border-0"
                    />
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </div>

      <Dialog
        open={Boolean(rotateTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setRotateTarget(null)
            setRotateValue('')
            setRotateError(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rotate secret · {rotateTarget?.name}</DialogTitle>
            <DialogDescription>
              Enter a new value. The existing secret value is never returned by the server and cannot be shown.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={Boolean(rotateError)}>
              <FieldLabel htmlFor={rotateValueId}>New secret value</FieldLabel>
              <MaskedSecretInput
                id={rotateValueId}
                value={rotateValue}
                aria-invalid={Boolean(rotateError)}
                onChange={(v) => {
                  setRotateValue(v)
                  if (rotateError) setRotateError(null)
                }}
                placeholder="Enter new secret value"
              />
              <FieldDescription>
                Applications will receive the new encrypted version upon next deployment.
              </FieldDescription>
              {rotateError ? <FieldError>{rotateError}</FieldError> : null}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRotateTarget(null)}>
              Cancel
            </Button>
            <Button type="button" onClick={confirmRotate}>
              <RefreshCw data-icon="inline-start" />
              Rotate secret
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(accessTarget)}
        onOpenChange={(open) => {
          if (!open) setAccessTarget(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Scoped access · {accessTarget?.name}</DialogTitle>
            <DialogDescription>
              Select platform roles authorized to mount or inject this secret into deployments. Secret values remain hidden in the UI.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            {ACCESS_ROLES.map((role) => {
              const checked = accessRoles.includes(role)
              return (
                <label
                  key={role}
                  className="flex cursor-pointer items-center justify-between rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted/50 transition-colors"
                >
                  <span className="font-medium text-foreground">{role}</span>
                  <input
                    type="checkbox"
                    checked={checked}
                    className="size-4 accent-primary rounded cursor-pointer"
                    onChange={() => {
                      setAccessRoles((prev) =>
                        checked ? prev.filter((r) => r !== role) : [...prev, role],
                      )
                    }}
                  />
                </label>
              )
            })}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setAccessTarget(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              onClick={() => {
                if (!accessTarget) return
                if (accessRoles.length === 0) {
                  toast.error('Select at least one role with access')
                  return
                }
                setSecrets((prev) =>
                  prev.map((secret) =>
                    secret.id === accessTarget.id
                      ? { ...secret, accessRoles: [...accessRoles] }
                      : secret,
                  ),
                )
                toast.success(`Access policy updated for ${accessTarget.name}`)
                setAccessTarget(null)
              }}
            >
              Save access
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {deleteTarget ? (
        <DestructiveConfirmDialog
          open={Boolean(deleteTarget)}
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null)
          }}
          title={`Delete secret ${deleteTarget.name}?`}
          description="Applications referencing this secret will fail to deploy or run once this secret is removed from the platform key vault."
          confirmLabel="Delete secret"
          confirmationPhrase={deleteTarget.name}
          onConfirm={() => {
            setSecrets((prev) => prev.filter((s) => s.id !== deleteTarget.id))
            toast.success(`Secret ${deleteTarget.name} permanently deleted`)
            setDeleteTarget(null)
          }}
        />
      ) : null}
    </div>
  )
}
