'use client'

import { useId, useMemo, useRef, useState } from 'react'
import {
  Braces,
  ClipboardPaste,
  FileUp,
  Layers,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
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
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { FilterBar } from '@/components/platform/filter-bar'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { EmptyState } from '@/components/platform/empty-state'
import { MaskedSecretInput, SecretField } from '@/components/platform/secret-field'
import { cn } from '@/lib/utils'
import {
  ENV_SCOPES,
  ENV_VAR_KIND_LABELS,
  detectOverrides,
  filterEnvVars,
  getEnvVarKind,
  parseEnvText,
  type EnvVarKind,
} from '@/lib/variables'
import { addVariableSchema } from '@/lib/validations/variable'
import type { EnvVarEntry } from '@/lib/types'

interface EnvironmentVariablesEditorProps {
  initialVariables: EnvVarEntry[]
  title?: string
  description?: string
  hideHierarchyBanner?: boolean
}

type Draft = {
  key: string
  value: string
  scope: EnvVarEntry['scope']
  source: string
  secret: boolean
}

const EMPTY_DRAFT: Draft = {
  key: '',
  value: '',
  scope: 'Application',
  source: 'daya-api',
  secret: false,
}

export function EnvironmentVariablesEditor({
  initialVariables,
  hideHierarchyBanner = false,
}: EnvironmentVariablesEditorProps) {
  const [variables, setVariables] = useState<EnvVarEntry[]>(() =>
    detectOverrides(initialVariables),
  )
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState('all')
  const [kind, setKind] = useState<EnvVarKind | 'all'>('all')

  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<EnvVarEntry | null>(null)
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT)
  const [errors, setErrors] = useState<{ key?: string; value?: string; source?: string }>({})

  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkText, setBulkText] = useState('')

  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')

  const [removeTarget, setRemoveTarget] = useState<EnvVarEntry | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const keyInputId = useId()
  const scopeSelectId = useId()
  const sourceInputId = useId()
  const secretSwitchId = useId()
  const valueInputId = useId()

  const filtered = useMemo(
    () => filterEnvVars(variables, { query, scope, kind }),
    [variables, query, scope, kind],
  )

  const counts = useMemo(() => {
    const base = { all: variables.length, inherited: 0, overridden: 0, 'application-specific': 0 }
    for (const entry of variables) {
      base[getEnvVarKind(entry)] += 1
    }
    return base
  }, [variables])

  function openCreate() {
    setEditing(null)
    setDraft(EMPTY_DRAFT)
    setErrors({})
    setEditorOpen(true)
  }

  function openEdit(entry: EnvVarEntry) {
    setEditing(entry)
    setDraft({
      key: entry.key,
      // Secrets are write-only — never repopulated from the server.
      value: entry.secret ? '' : entry.value,
      scope: entry.scope,
      source: entry.source,
      secret: entry.secret,
    })
    setErrors({})
    setEditorOpen(true)
  }

  function saveVariable() {
    const result = addVariableSchema.safeParse(draft)
    if (!result.success) {
      const fieldErrors: { key?: string; value?: string; source?: string } = {}
      for (const issue of result.error.issues) {
        const path = issue.path[0] as 'key' | 'value' | 'source'
        if (path && !fieldErrors[path]) {
          fieldErrors[path] = issue.message
        }
      }
      setErrors(fieldErrors)
      return
    }

    if (draft.secret && editing && draft.value.trim() === '') {
      setErrors({ value: 'Enter a new secret value (existing values are never shown or reused)' })
      return
    }

    const key = draft.key.trim()
    const value = draft.secret ? '••••••••••••' : draft.value

    setVariables((prev) => {
      let updated: EnvVarEntry[]
      if (editing) {
        updated = prev.map((entry) =>
          entry.id === editing.id
            ? {
                ...entry,
                key,
                value: draft.secret && draft.value.trim() === '' ? entry.value : value,
                scope: draft.scope,
                source: draft.source,
                secret: draft.secret,
              }
            : entry,
        )
      } else {
        const newItem: EnvVarEntry = {
          id: `ev-${Date.now()}`,
          key,
          value,
          secret: draft.secret,
          scope: draft.scope,
          source: draft.source,
          overridden: false,
        }
        updated = [newItem, ...prev]
      }
      return detectOverrides(updated)
    })

    toast.success(editing ? `Updated ${key}` : `Added ${key}`)
    setEditorOpen(false)
  }

  function applyParsedRows(rows: ReturnType<typeof parseEnvText>, label: string) {
    if (rows.length === 0) {
      toast.error('No valid KEY=value lines found')
      return
    }
    setVariables((prev) => {
      const next = [...prev]
      for (const row of rows) {
        const existing = next.findIndex(
          (entry) => entry.key === row.key && entry.scope === 'Application',
        )
        const item: EnvVarEntry = {
          id: existing >= 0 ? next[existing].id : `ev-${Date.now()}-${row.key}`,
          key: row.key,
          value: row.secret ? '••••••••••••' : row.value,
          secret: row.secret,
          scope: 'Application',
          source: 'daya-api',
          overridden: false,
        }
        if (existing >= 0) {
          next[existing] = item
        } else {
          next.unshift(item)
        }
      }
      return detectOverrides(next)
    })
    toast.success(`${label}: ${rows.length} variable${rows.length === 1 ? '' : 's'} imported`)
  }

  return (
    <div className="flex flex-col gap-4">
      {!hideHierarchyBanner ? (
        <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <Layers className="size-4 shrink-0 text-primary" />
            <span className="text-xs font-medium text-foreground">Hierarchy Resolution</span>
          </div>
          <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
            <Badge variant="outline" className="text-[10px]">1. Organization</Badge>
            <span>→</span>
            <Badge variant="outline" className="text-[10px]">2. Project</Badge>
            <span>→</span>
            <Badge variant="outline" className="text-[10px]">3. Environment</Badge>
            <span>→</span>
            <Badge variant="secondary" className="text-[10px]">4. Application</Badge>
            <span className="ml-1 text-[11px] opacity-75">(lower scopes override parent scopes)</span>
          </div>
        </div>
      ) : null}

      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap gap-1.5">
          {(
            [
              ['all', 'All'],
              ['inherited', 'Inherited'],
              ['overridden', 'Overridden'],
              ['application-specific', 'Application-specific'],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              onClick={() => setKind(id)}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors',
                kind === id
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {label}
              <span className="tabular text-[10px] opacity-70">
                {id === 'all' ? counts.all : counts[id]}
              </span>
            </button>
          ))}
        </div>

        <FilterBar
          end={
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" variant="outline" onClick={() => setBulkOpen(true)}>
                <ClipboardPaste data-icon="inline-start" />
                Bulk paste
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={() => setImportOpen(true)}>
                <FileUp data-icon="inline-start" />
                .env import
              </Button>
              <Button type="button" size="sm" onClick={openCreate}>
                <Plus data-icon="inline-start" />
                Add variable
              </Button>
            </div>
          }
        >
          <InputGroup className="max-w-xs">
            <InputGroupInput
              placeholder="Search keys, sources, values…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
          </InputGroup>
          <Select value={scope} onValueChange={(v) => setScope(v ?? 'all')}>
            <SelectTrigger className="w-44">
              <SelectValue placeholder="Scope" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All scopes</SelectItem>
              {ENV_SCOPES.map((s) => (
                <SelectItem key={s} value={s}>
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FilterBar>
      </div>

      <div className="rounded-lg border border-border overflow-hidden">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Key</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Value</TableHead>
                <TableHead className="w-10 text-right">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((entry) => {
                const entryKind = getEnvVarKind(entry)
                return (
                  <TableRow key={entry.id}>
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        <span className="font-mono text-xs font-semibold">{entry.key}</span>
                        {entry.secret ? (
                          <Badge variant="outline" className="text-[10px] text-amber-600 dark:text-amber-400 border-amber-500/30">
                            Secret
                          </Badge>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={entryKind === 'overridden' ? 'secondary' : entryKind === 'inherited' ? 'outline' : 'default'}
                        className="text-[10px]"
                      >
                        {ENV_VAR_KIND_LABELS[entryKind]}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary" className="text-[10px]">
                        {entry.scope}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{entry.source}</TableCell>
                    <TableCell>
                      {entry.secret ? (
                        <SecretField value={entry.value} neverReveal className="w-44" />
                      ) : (
                        <span className="font-mono text-xs text-muted-foreground break-all">{entry.value}</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={`Actions for ${entry.key}`}
                            />
                          }
                        >
                          <MoreHorizontal />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(entry)}>
                            <Pencil />
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setRemoveTarget(entry)}
                          >
                            <Trash2 />
                            Remove
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                )
              })}
              {filtered.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="py-12">
                    <EmptyState
                      icon={Braces}
                      title="No variables found"
                      description={
                        query || scope !== 'all' || kind !== 'all'
                          ? 'No environment variables match your filter criteria.'
                          : 'No environment variables configured yet.'
                      }
                      action={
                        query || scope !== 'all' || kind !== 'all' ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setQuery('')
                              setScope('all')
                              setKind('all')
                            }}
                          >
                            Reset filters
                          </Button>
                        ) : (
                          <Button size="sm" onClick={openCreate}>
                            <Plus data-icon="inline-start" />
                            Add variable
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

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? `Edit variable ${editing.key}` : 'Add variable'}</DialogTitle>
            <DialogDescription>
              Scope hierarchy: Organization → Project → Environment → Application. Secret values
              are write-only and never revealed.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={Boolean(errors.key)}>
              <FieldLabel htmlFor={keyInputId}>Key</FieldLabel>
              <Input
                id={keyInputId}
                className="font-mono"
                value={draft.key}
                aria-invalid={Boolean(errors.key)}
                onChange={(e) => {
                  setDraft((d) => ({ ...d, key: e.target.value }))
                  if (errors.key) setErrors((prev) => ({ ...prev, key: undefined }))
                }}
                placeholder="DATABASE_URL"
              />
              {errors.key ? <FieldError>{errors.key}</FieldError> : null}
            </Field>

            <Field>
              <FieldLabel htmlFor={scopeSelectId}>Scope</FieldLabel>
              <Select
                value={draft.scope}
                onValueChange={(v) =>
                  setDraft((d) => ({ ...d, scope: (v as EnvVarEntry['scope']) ?? 'Application' }))
                }
              >
                <SelectTrigger id={scopeSelectId} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ENV_SCOPES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field data-invalid={Boolean(errors.source)}>
              <FieldLabel htmlFor={sourceInputId}>Source</FieldLabel>
              <Input
                id={sourceInputId}
                value={draft.source}
                aria-invalid={Boolean(errors.source)}
                onChange={(e) => {
                  setDraft((d) => ({ ...d, source: e.target.value }))
                  if (errors.source) setErrors((prev) => ({ ...prev, source: undefined }))
                }}
                placeholder="daya-api or Production"
              />
              <FieldDescription>Application, environment, or project name declaring this variable.</FieldDescription>
              {errors.source ? <FieldError>{errors.source}</FieldError> : null}
            </Field>

            <Field className="flex flex-row items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5">
              <div className="flex flex-col gap-0.5">
                <FieldLabel htmlFor={secretSwitchId} className="cursor-pointer">Secret variable</FieldLabel>
                <FieldDescription>Mask value. DeployCore never sends existing plaintext to UI.</FieldDescription>
              </div>
              <Switch
                id={secretSwitchId}
                checked={draft.secret}
                onCheckedChange={(checked) =>
                  setDraft((d) => ({ ...d, secret: checked, value: checked ? '' : d.value }))
                }
              />
            </Field>

            <Field data-invalid={Boolean(errors.value)}>
              <FieldLabel htmlFor={valueInputId}>Value</FieldLabel>
              {draft.secret ? (
                <MaskedSecretInput
                  id={valueInputId}
                  value={draft.value}
                  onChange={(value) => {
                    setDraft((d) => ({ ...d, value }))
                    if (errors.value) setErrors((prev) => ({ ...prev, value: undefined }))
                  }}
                  aria-invalid={Boolean(errors.value)}
                  placeholder={editing ? 'Enter new secret value (existing never shown)' : 'Enter secret value'}
                />
              ) : (
                <Input
                  id={valueInputId}
                  className="font-mono"
                  value={draft.value}
                  aria-invalid={Boolean(errors.value)}
                  onChange={(e) => {
                    setDraft((d) => ({ ...d, value: e.target.value }))
                    if (errors.value) setErrors((prev) => ({ ...prev, value: undefined }))
                  }}
                  placeholder="value"
                />
              )}
              {errors.value ? <FieldError>{errors.value}</FieldError> : null}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setEditorOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={saveVariable}>
              Save variable
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={bulkOpen} onOpenChange={setBulkOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Bulk paste variables</DialogTitle>
            <DialogDescription>
              Paste KEY=value pairs. Applied at Application scope. Keys matching SECRET, TOKEN, PASSWORD, or KEY are automatically marked as secrets.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={bulkText}
            onChange={(e) => setBulkText(e.target.value)}
            className="min-h-40 font-mono text-xs"
            placeholder={'API_BASE_URL=https://api.example.com\nDATABASE_PORT=5432\nSTRIPE_API_KEY=sk_live_...'}
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setBulkOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              onClick={() => {
                applyParsedRows(parseEnvText(bulkText), 'Bulk paste')
                setBulkOpen(false)
                setBulkText('')
              }}
            >
              Import variables
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>.env file import</DialogTitle>
            <DialogDescription>
              Upload a .env file or paste its content. Comments (#) and empty lines are skipped.
            </DialogDescription>
          </DialogHeader>
          <input
            ref={fileRef}
            type="file"
            accept=".env,text/plain"
            className="hidden"
            onChange={async (e) => {
              const file = e.target.files?.[0]
              if (!file) return
              const text = await file.text()
              setImportText(text)
              e.target.value = ''
            }}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            onClick={() => fileRef.current?.click()}
          >
            <FileUp data-icon="inline-start" />
            Select .env file
          </Button>
          <Textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            className="min-h-40 font-mono text-xs"
            placeholder="# Paste .env file contents here..."
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setImportOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              onClick={() => {
                applyParsedRows(parseEnvText(importText), '.env import')
                setImportOpen(false)
                setImportText('')
              }}
            >
              Import variables
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {removeTarget ? (
        <DestructiveConfirmDialog
          open={Boolean(removeTarget)}
          onOpenChange={(open) => {
            if (!open) setRemoveTarget(null)
          }}
          title={`Remove ${removeTarget.key}?`}
          description={`This removes ${removeTarget.key} from the ${removeTarget.scope} scope. Inherited values from higher scopes (if any) will reactivate.`}
          confirmLabel="Remove variable"
          confirmationPhrase={removeTarget.key}
          onConfirm={() => {
            setVariables((prev) => detectOverrides(prev.filter((entry) => entry.id !== removeTarget.id)))
            toast.success(`${removeTarget.key} removed`)
            setRemoveTarget(null)
          }}
        />
      ) : null}
    </div>
  )
}
