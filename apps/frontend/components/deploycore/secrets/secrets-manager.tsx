'use client'

import { useMemo, useState } from 'react'
import { KeyRound, MoreHorizontal, Plus, RefreshCw, Shield } from 'lucide-react'
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
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { MaskedSecretInput } from '@/components/platform/secret-field'
import type { SecretItem } from '@/lib/types'

const SECRET_SCOPES = ['Organization', 'Project', 'Application'] as const
const ACCESS_ROLES = ['Owner', 'Admin', 'Developer', 'Viewer'] as const

interface SecretsManagerProps {
  initialSecrets: SecretItem[]
}

export function SecretsManager({ initialSecrets }: SecretsManagerProps) {
  const [secrets, setSecrets] = useState(initialSecrets)
  const [createOpen, setCreateOpen] = useState(false)
  const [rotateTarget, setRotateTarget] = useState<SecretItem | null>(null)
  const [accessTarget, setAccessTarget] = useState<SecretItem | null>(null)

  const [name, setName] = useState('')
  const [scope, setScope] = useState<(typeof SECRET_SCOPES)[number]>('Application')
  const [apps, setApps] = useState('')
  const [description, setDescription] = useState('')
  /** Always empty — never populated from server. */
  const [newValue, setNewValue] = useState('')
  const [createError, setCreateError] = useState<string | null>(null)

  const [rotateValue, setRotateValue] = useState('')
  const [rotateError, setRotateError] = useState<string | null>(null)

  const [accessRoles, setAccessRoles] = useState<string[]>([])

  function resetCreate() {
    setName('')
    setScope('Application')
    setApps('')
    setDescription('')
    setNewValue('')
    setCreateError(null)
  }

  function createSecret() {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name.trim())) {
      setCreateError('Use a valid SECRET_NAME')
      return
    }
    if (!newValue.trim()) {
      setCreateError('Enter a secret value')
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
    toast.success(`${item.name} created`)
    setCreateOpen(false)
    resetCreate()
  }

  function confirmRotate() {
    if (!rotateTarget) return
    if (!rotateValue.trim()) {
      setRotateError('Enter a new secret value — the current value is never shown')
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
    toast.success(`${rotateTarget.name} rotated`)
    setRotateTarget(null)
    setRotateValue('')
    setRotateError(null)
  }

  const sorted = useMemo(
    () => [...secrets].sort((a, b) => a.name.localeCompare(b.name)),
    [secrets],
  )

  return (
    <div className="flex flex-col">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
        <p className="text-xs text-muted-foreground">
          Metadata only — secret values are never listed or repopulated from the server.
        </p>
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
              <DialogTitle>New secret</DialogTitle>
              <DialogDescription>
                Values are write-only. After save, DeployCore never returns the plaintext to the UI.
              </DialogDescription>
            </DialogHeader>
            <FieldGroup>
              <Field data-invalid={Boolean(createError)}>
                <FieldLabel htmlFor="sec-name">Name</FieldLabel>
                <Input
                  id="sec-name"
                  className="font-mono"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="API_SIGNING_KEY"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="sec-scope">Scope</FieldLabel>
                <Select
                  value={scope}
                  onValueChange={(v) => setScope((v as typeof scope) ?? 'Application')}
                >
                  <SelectTrigger id="sec-scope" className="w-full">
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
                <FieldLabel htmlFor="sec-apps">Applications</FieldLabel>
                <Input
                  id="sec-apps"
                  value={apps}
                  onChange={(e) => setApps(e.target.value)}
                  placeholder="daya-api, daya-web"
                />
                <FieldDescription>Comma-separated application names with access.</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="sec-desc">Description</FieldLabel>
                <Textarea
                  id="sec-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className="min-h-16"
                />
              </Field>
              <Field data-invalid={Boolean(createError)}>
                <FieldLabel htmlFor="sec-value">Value</FieldLabel>
                <MaskedSecretInput
                  id="sec-value"
                  value={newValue}
                  onChange={setNewValue}
                  placeholder="Enter secret value"
                />
                {createError ? <FieldError>{createError}</FieldError> : null}
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
      </div>

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
                  <span className="font-mono text-xs font-medium">{secret.name}</span>
                  {secret.description ? (
                    <span className="text-[11px] text-muted-foreground">{secret.description}</span>
                  ) : null}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="secondary">{secret.scope}</Badge>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {secret.applications.map((app) => (
                    <Badge key={app} variant="outline" className="font-mono text-[10px]">
                      {app}
                    </Badge>
                  ))}
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
                  <span className="text-muted-foreground">by {secret.updatedBy}</span>
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
                      Rotate
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
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

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
            <DialogTitle>Rotate {rotateTarget?.name}</DialogTitle>
            <DialogDescription>
              Enter a new value. The current secret is never returned by the server and cannot be
              shown or edited in place.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={Boolean(rotateError)}>
              <FieldLabel htmlFor="rotate-value">New value</FieldLabel>
              <MaskedSecretInput
                id="rotate-value"
                value={rotateValue}
                onChange={setRotateValue}
                placeholder="Enter new secret value"
              />
              <FieldDescription>
                Applications will receive the new value on next deploy / secret sync.
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
              Roles allowed to use this secret in deployments. Values remain inaccessible in the UI.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            {ACCESS_ROLES.map((role) => {
              const checked = accessRoles.includes(role)
              return (
                <label
                  key={role}
                  className="flex cursor-pointer items-center justify-between rounded-lg border border-border px-3 py-2 text-sm"
                >
                  <span>{role}</span>
                  <input
                    type="checkbox"
                    checked={checked}
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
                setSecrets((prev) =>
                  prev.map((secret) =>
                    secret.id === accessTarget.id
                      ? { ...secret, accessRoles: [...accessRoles] }
                      : secret,
                  ),
                )
                toast.success(`Access updated for ${accessTarget.name}`)
                setAccessTarget(null)
              }}
            >
              Save access
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
