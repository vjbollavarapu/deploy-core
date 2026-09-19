'use client'

import { useMemo, useRef, useState } from 'react'
import {
  FileUp,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  ClipboardPaste,
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
import { MaskedSecretInput, SecretField } from '@/components/platform/secret-field'
import { cn } from '@/lib/utils'
import {
  ENV_SCOPES,
  ENV_VAR_KIND_LABELS,
  filterEnvVars,
  getEnvVarKind,
  parseEnvText,
  type EnvVarKind,
} from '@/lib/variables'
import type { EnvVarEntry } from '@/lib/types'

interface EnvironmentVariablesEditorProps {
  initialVariables: EnvVarEntry[]
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

export function EnvironmentVariablesEditor({ initialVariables }: EnvironmentVariablesEditorProps) {
  const [variables, setVariables] = useState(initialVariables)
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState('all')
  const [kind, setKind] = useState<EnvVarKind | 'all'>('all')
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<EnvVarEntry | null>(null)
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT)
  const [formError, setFormError] = useState<string | null>(null)
  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkText, setBulkText] = useState('')
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')
  const [removeTarget, setRemoveTarget] = useState<EnvVarEntry | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

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
    setFormError(null)
    setEditorOpen(true)
  }

  function openEdit(entry: EnvVarEntry) {
    setEditing(entry)
    setDraft({
      key: entry.key,
      // Secrets are never repopulated from the server — leave blank for re-entry.
      value: entry.secret ? '' : entry.value,
      scope: entry.scope,
      source: entry.source,
      secret: entry.secret,
    })
    setFormError(null)
    setEditorOpen(true)
  }

  function saveVariable() {
    const key = draft.key.trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      setFormError('Use a valid KEY_NAME')
      return
    }
    if (!draft.secret && draft.value.trim() === '') {
      setFormError('Value is required')
      return
    }
    if (draft.secret && !editing && draft.value.trim() === '') {
      setFormError('Secret value is required')
      return
    }
    if (draft.secret && editing && draft.value.trim() === '') {
      setFormError('Enter a new secret value — existing values are never shown or reused')
      return
    }

    if (editing) {
      setVariables((prev) =>
        prev.map((entry) =>
          entry.id === editing.id
            ? {
                ...entry,
                key,
                value: draft.secret ? '••••••••••••' : draft.value,
                scope: draft.scope,
                source: draft.source,
                secret: draft.secret,
                overridden: draft.scope !== 'Organization' && key === 'LOG_LEVEL',
              }
            : entry,
        ),
      )
      toast.success(`Updated ${key}`)
    } else {
      setVariables((prev) => [
        {
          id: `ev-${Date.now()}`,
          key,
          value: draft.secret ? '••••••••••••' : draft.value,
          secret: draft.secret,
          scope: draft.scope,
          source: draft.source,
          overridden: false,
        },
        ...prev,
      ])
      toast.success(`Added ${key}`)
    }
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
        if (existing >= 0) next[existing] = item
        else next.unshift(item)
      }
      return next
    })
    toast.success(`${label}: ${rows.length} variable${rows.length === 1 ? '' : 's'}`)
  }

  return (
    <div className="flex flex-col gap-0">
      <div className="flex flex-col gap-3 p-4">
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
              placeholder="Search keys…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
          </InputGroup>
          <Select value={scope} onValueChange={(v) => setScope(v ?? 'all')}>
            <SelectTrigger className="w-40">
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

      <div className="border-t border-border">
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
                      <span className="font-mono text-xs font-medium">{entry.key}</span>
                      {entry.secret ? (
                        <Badge variant="outline" className="text-[10px]">
                          Secret
                        </Badge>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={entryKind === 'overridden' ? 'secondary' : 'outline'}
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
                      <SecretField value={entry.value} neverReveal className="w-48" />
                    ) : (
                      <span className="font-mono text-xs text-muted-foreground">{entry.value}</span>
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
                <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                  No variables match your filters.
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? 'Edit variable' : 'Add variable'}</DialogTitle>
            <DialogDescription>
              Hierarchy: Organization → Project → Environment → Application. Secret values are
              never loaded from the server when editing.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={Boolean(formError)}>
              <FieldLabel htmlFor="ev-key">Key</FieldLabel>
              <Input
                id="ev-key"
                className="font-mono"
                value={draft.key}
                onChange={(e) => setDraft((d) => ({ ...d, key: e.target.value }))}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="ev-scope">Scope</FieldLabel>
              <Select
                value={draft.scope}
                onValueChange={(v) =>
                  setDraft((d) => ({ ...d, scope: (v as EnvVarEntry['scope']) ?? 'Application' }))
                }
              >
                <SelectTrigger id="ev-scope" className="w-full">
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
            <Field>
              <FieldLabel htmlFor="ev-source">Source</FieldLabel>
              <Input
                id="ev-source"
                value={draft.source}
                onChange={(e) => setDraft((d) => ({ ...d, source: e.target.value }))}
              />
            </Field>
            <Field className="flex flex-row items-center justify-between gap-3 rounded-lg border border-border px-3 py-2">
              <div>
                <FieldLabel htmlFor="ev-secret">Secret</FieldLabel>
                <FieldDescription>Mask value and never echo from server on edit.</FieldDescription>
              </div>
              <Switch
                id="ev-secret"
                checked={draft.secret}
                onCheckedChange={(checked) =>
                  setDraft((d) => ({ ...d, secret: checked, value: checked ? '' : d.value }))
                }
              />
            </Field>
            <Field data-invalid={Boolean(formError)}>
              <FieldLabel htmlFor="ev-value">Value</FieldLabel>
              {draft.secret ? (
                <MaskedSecretInput
                  id="ev-value"
                  value={draft.value}
                  onChange={(value) => setDraft((d) => ({ ...d, value }))}
                  placeholder={editing ? 'Enter new value (existing never shown)' : 'Enter secret value'}
                />
              ) : (
                <Input
                  id="ev-value"
                  className="font-mono"
                  value={draft.value}
                  onChange={(e) => setDraft((d) => ({ ...d, value: e.target.value }))}
                />
              )}
              {formError ? <FieldError>{formError}</FieldError> : null}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setEditorOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={saveVariable}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={bulkOpen} onOpenChange={setBulkOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Bulk paste</DialogTitle>
            <DialogDescription>
              Paste KEY=value lines. Applied at Application scope. Lines with SECRET/PASSWORD/TOKEN
              keys are treated as secrets.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={bulkText}
            onChange={(e) => setBulkText(e.target.value)}
            className="min-h-40 font-mono text-xs"
            placeholder={'API_BASE_URL=https://api.example.com\nFEATURE_FLAG=true'}
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
              Import lines
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>.env import</DialogTitle>
            <DialogDescription>
              Upload a .env file or paste its contents. Comments and blank lines are ignored.
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
            Choose .env file
          </Button>
          <Textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            className="min-h-40 font-mono text-xs"
            placeholder="# pasted .env contents"
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
              Import
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
          description="This removes the variable from the selected scope. Inherited values from parent scopes remain."
          confirmLabel="Remove"
          confirmationPhrase={removeTarget.key}
          onConfirm={() => {
            setVariables((prev) => prev.filter((entry) => entry.id !== removeTarget.id))
            toast.success(`${removeTarget.key} removed`)
            setRemoveTarget(null)
          }}
        />
      ) : null}
    </div>
  )
}
