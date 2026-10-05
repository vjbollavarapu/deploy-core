'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ApplicationSourceValue } from '@/components/deploycore/applications/application-source-value'
import { useReloadProductionApplication } from '@/components/deploycore/applications/production-application-shell'
import { useApplicationSourceRows } from '@/components/deploycore/applications/use-application-source-display'
import { DetailList } from '@/components/platform/detail-list'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { apiClient } from '@/lib/api'
import {
  acceptEditorConnections,
  acceptEditorRepositories,
  initialSourceDraft,
  repositoryIdForCurrentUrl,
  runApplicationSourceSave,
  shouldLeaveSourceEditor,
  sourceDraftAfterMode,
  type EditorConnectionScope,
  type EditorRepositoryScope,
  type EditorSourceMode,
  type SourceEditorDraft,
} from '@/lib/applications/application-source-editor'
import type { ApplicationSourceSnapshot } from '@/lib/applications/application-source-update'
import {
  connectionFromOption,
  connectionListMessage,
  GIT_CONNECTION_NONE,
  GIT_CONNECTION_NONE_ACTIVE,
  GIT_CONNECTION_NONE_HINT,
  GIT_REPOSITORY_NONE,
  GIT_REPOSITORY_NONE_HINT,
  loadWizardGitConnections,
  loadWizardRepositories,
  repositoryListMessage,
  wizardConnectionInputFromMapped,
  wizardConnectionOptions,
  wizardRepositoryOptions,
  type GitSourceLoadPhase,
  type WizardConnectionOption,
  type WizardRepositoryOption,
} from '@/lib/applications/git-source-wizard'
import { useOrganization } from '@/lib/auth-context'
import type { ApplicationDetail } from '@/lib/control-plane/detail-read'
import { GIT_PROVIDERS_PATH } from '@/lib/github/callback'
import { mapWireGitConnection, type WireGitConnection, type WireGitRepository } from '@/lib/github/git-connection'
import { fetchGitConnections, fetchGitRepositories } from '@/lib/integrations'

const NO_CONNECTIONS: WizardConnectionOption[] = []
const NO_REPOSITORIES: WizardRepositoryOption[] = []

const MODES: { value: EditorSourceMode; label: string }[] = [
  { value: 'connected-git', label: 'Connected repository' },
  { value: 'public-git', label: 'Public Git' },
  { value: 'image', label: 'Image' },
  { value: 'compose', label: 'Compose' },
]

export function ApplicationSourceEditor({ application }: { application: ApplicationDetail }) {
  const { activeOrg } = useOrganization()
  const reload = useReloadProductionApplication()
  const sourceRows = useApplicationSourceRows(application, 'detail')
  const organizationId = activeOrg?.id ?? ''
  const current = snapshotOf(application)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<SourceEditorDraft>(() => initialSourceDraft(current))
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)
  const mounted = useRef(true)
  const organizationIdRef = useRef(organizationId)
  const [connectionLoad, setConnectionLoad] = useState<{
    scope: EditorConnectionScope
    phase: 'ready' | 'error'
    options: WizardConnectionOption[]
  } | null>(null)
  const [repositoryLoad, setRepositoryLoad] = useState<{
    scope: EditorRepositoryScope
    phase: 'ready' | 'error'
    options: WizardRepositoryOption[]
  } | null>(null)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  useEffect(() => {
    organizationIdRef.current = organizationId
  }, [organizationId])

  const connectionScope = { organizationId, applicationId: application.id }
  const connectionsCurrent =
    editing && draft.mode === 'connected-git' && organizationId && connectionLoad && acceptEditorConnections(connectionLoad.scope, connectionScope)
      ? connectionLoad
      : null
  const connectionPhase: GitSourceLoadPhase =
    !editing || draft.mode !== 'connected-git' || !organizationId ? 'idle' : connectionsCurrent ? connectionsCurrent.phase : 'loading'
  const connections = connectionsCurrent?.options ?? NO_CONNECTIONS
  const selectableConnection = connections.find((item) => item.id === draft.gitConnectionId && item.selectable) ?? null

  useEffect(() => {
    if (!editing || draft.mode !== 'connected-git' || !organizationId) return
    const scope = { organizationId, applicationId: application.id }
    let active = true
    void loadWizardGitConnections(organizationId, (page) => fetchGitConnections(organizationId, page))
      .then((wires) => {
        if (!active) return
        setConnectionLoad({
          scope,
          phase: 'ready',
          options: wizardConnectionOptions(wires.map(connectionInput), organizationId),
        })
      })
      .catch(() => {
        if (!active) return
        setConnectionLoad({ scope, phase: 'error', options: [] })
      })
    return () => {
      active = false
    }
  }, [application.id, draft.mode, editing, organizationId])

  useEffect(() => {
    if (!editing || draft.mode !== 'connected-git' || !organizationId || !selectableConnection) return
    const scope = { organizationId, applicationId: application.id, connectionId: selectableConnection.id }
    const connection = connectionFromOption(selectableConnection)
    let active = true
    void loadWizardRepositories((page) => fetchGitRepositories(scope.connectionId, page))
      .then((wires) => {
        if (!active) return
        const options = wizardRepositoryOptions({
          organizationId,
          connection,
          repositories: wires.map(repositoryInput),
        })
        setRepositoryLoad({ scope, phase: 'ready', options })
        setDraft((existing) => {
          if (existing.mode !== 'connected-git' || existing.gitConnectionId !== scope.connectionId || existing.repositoryId) return existing
          const repositoryId = repositoryIdForCurrentUrl(application.repositoryUrl, options.map((option) => ({
            id: option.id,
            selectable: option.selectable,
            cloneUrl: option.selection?.cloneUrl,
          })))
          return repositoryId ? { ...existing, repositoryId } : existing
        })
      })
      .catch(() => {
        if (!active) return
        setRepositoryLoad({ scope, phase: 'error', options: [] })
      })
    return () => {
      active = false
    }
  }, [application.id, application.repositoryUrl, draft.mode, editing, organizationId, selectableConnection])

  const repositoryScope = { organizationId, applicationId: application.id, connectionId: selectableConnection?.id ?? '' }
  const repositoriesCurrent =
    selectableConnection && repositoryLoad && acceptEditorRepositories(repositoryLoad.scope, repositoryScope) ? repositoryLoad : null
  const repositoryPhase: GitSourceLoadPhase = !selectableConnection ? 'idle' : repositoriesCurrent ? repositoriesCurrent.phase : 'loading'
  const repositories = repositoriesCurrent?.options ?? NO_REPOSITORIES
  const selectedRepository = repositories.find((item) => item.id === draft.repositoryId && item.selectable)?.selection ?? null
  const connectionMessage = connectionListMessage({
    phase: connectionPhase,
    visibleCount: connections.length,
    selectableCount: connections.filter((item) => item.selectable).length,
  })
  const repositoryMessage = repositoryListMessage({
    connectionSelected: Boolean(selectableConnection),
    phase: repositoryPhase,
    repositoryCount: repositories.length,
  })
  const currentConnectionUnavailable =
    draft.mode === 'connected-git' &&
    Boolean(current.gitConnectionId) &&
    connectionPhase === 'ready' &&
    !selectableConnection &&
    !connections.some((item) => item.id === current.gitConnectionId && item.selectable)
  const connectionItems = useMemo(() => {
    const items: Record<string, string> = {}
    for (const connection of connections) items[connection.id] = connection.label
    return items
  }, [connections])
  const repositoryItems = useMemo(() => {
    const items: Record<string, string> = {}
    for (const repository of repositories) items[repository.id] = repository.label
    return items
  }, [repositories])
  const modeItems = useMemo(() => {
    const items: Record<string, string> = {}
    for (const mode of MODES) items[mode.value] = mode.label
    return items
  }, [])

  function beginEdit() {
    setDraft(initialSourceDraft(snapshotOf(application)))
    setError(null)
    setEditing(true)
  }

  function cancelEdit() {
    setDraft(initialSourceDraft(snapshotOf(application)))
    setError(null)
    setEditing(false)
  }

  async function save() {
    if (savingRef.current) return
    savingRef.current = true
    setSaving(true)
    setError(null)
    const applicationId = application.id
    try {
      const result = await runApplicationSourceSave({
        current: snapshotOf(application),
        draft,
        organizationId,
        applicationId,
        connection: selectableConnection ? connectionFromOption(selectableConnection) : null,
        repository: selectedRepository ?? null,
        patch: async (config) => {
          if (organizationIdRef.current !== organizationId) {
            throw new Error('The source configuration could not be saved.')
          }
          await apiClient.patch(`/applications/${applicationId}`, { config })
        },
        reload: async () => {
          const refreshed = await reload?.()
          return refreshed && refreshed.id === applicationId ? { id: refreshed.id } : null
        },
      })
      if (!mounted.current) return
      if (!shouldLeaveSourceEditor(result)) {
        setError(result.ok ? null : result.message)
        return
      }
      toast.success('Source configuration saved')
      setEditing(false)
    } finally {
      savingRef.current = false
      if (mounted.current) setSaving(false)
    }
  }

  return (
    <Card size="sm">
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>Source</CardTitle>
          <CardDescription>How this application is built. Saving does not deploy it.</CardDescription>
        </div>
        {editing ? null : (
          <Button type="button" size="sm" variant="outline" onClick={beginEdit}>
            Edit source
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {editing ? (
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="source-mode">Source</FieldLabel>
              <Select
                items={modeItems}
                value={draft.mode}
                disabled={saving}
                onValueChange={(value) => {
                  if (!value) return
                  setDraft(sourceDraftAfterMode(current, draft, value as EditorSourceMode))
                  setError(null)
                }}
              >
                <SelectTrigger id="source-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODES.map((mode) => (
                    <SelectItem key={mode.value} value={mode.value} label={mode.label}>
                      {mode.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {draft.mode === 'connected-git' ? (
              <>
                <Field>
                  <FieldLabel htmlFor="source-connection">Git connection</FieldLabel>
                  <Select
                    items={connectionItems}
                    value={selectableConnection?.id ?? null}
                    disabled={saving || connectionPhase === 'loading'}
                    onValueChange={(value) => {
                      if (!value || saving) return
                      const next = connections.find((item) => item.id === value && item.selectable)
                      if (!next) return
                      setDraft({ ...draft, gitConnectionId: next.id, repositoryId: '' })
                    }}
                  >
                    <SelectTrigger id="source-connection" className="w-full">
                      <SelectValue placeholder={connectionPhase === 'loading' ? 'Loading Git connections…' : 'Select a Git connection'} />
                    </SelectTrigger>
                    <SelectContent>
                      {connections.map((connection) => (
                        <SelectItem key={connection.id} value={connection.id} label={connection.label} disabled={!connection.selectable}>
                          {connection.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {currentConnectionUnavailable ? (
                    <FieldDescription>The current Git connection is not available. Choose an active connection and repository.</FieldDescription>
                  ) : null}
                  {connectionMessage ? (
                    <FieldDescription role={connectionPhase === 'error' ? 'alert' : 'status'}>
                      {connectionMessage}
                      {connectionMessage === GIT_CONNECTION_NONE || connectionMessage === GIT_CONNECTION_NONE_ACTIVE ? (
                        <>
                          {' '}
                          {GIT_CONNECTION_NONE_HINT}{' '}
                          <a href={GIT_PROVIDERS_PATH} className="text-primary underline-offset-4 hover:underline">
                            Open Git Providers
                          </a>
                          .
                        </>
                      ) : null}
                    </FieldDescription>
                  ) : null}
                </Field>
                <Field>
                  <FieldLabel htmlFor="source-repository">Repository</FieldLabel>
                  <Select
                    items={repositoryItems}
                    value={selectedRepository?.id ?? null}
                    disabled={saving || !selectableConnection || repositoryPhase === 'loading'}
                    onValueChange={(value) => {
                      if (!value || saving) return
                      const next = repositories.find((item) => item.id === value && item.selectable)
                      if (!next) return
                      setDraft({ ...draft, repositoryId: next.id })
                    }}
                  >
                    <SelectTrigger id="source-repository" className="w-full">
                      <SelectValue placeholder={repositoryPhase === 'loading' ? 'Loading repositories…' : 'Select a repository'} />
                    </SelectTrigger>
                    <SelectContent>
                      {repositories.map((repository) => (
                        <SelectItem key={repository.id} value={repository.id} label={repository.label} disabled={!repository.selectable}>
                          {repository.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {repositoryMessage ? (
                    <FieldDescription role={repositoryPhase === 'error' ? 'alert' : 'status'}>
                      {repositoryMessage}
                      {repositoryMessage === GIT_REPOSITORY_NONE ? (
                        <>
                          {' '}
                          {GIT_REPOSITORY_NONE_HINT}{' '}
                          <a href={GIT_PROVIDERS_PATH} className="text-primary underline-offset-4 hover:underline">
                            Open Git Providers
                          </a>
                          .
                        </>
                      ) : null}
                    </FieldDescription>
                  ) : null}
                </Field>
              </>
            ) : null}
            {draft.mode === 'public-git' || draft.mode === 'compose' ? (
              <Field>
                <FieldLabel htmlFor="source-repository-url">{draft.mode === 'compose' ? 'Compose repository' : 'Repository URL'}</FieldLabel>
                <Input
                  id="source-repository-url"
                  className="font-mono"
                  value={draft.repositoryUrl}
                  disabled={saving}
                  onChange={(event) => setDraft({ ...draft, repositoryUrl: event.target.value })}
                />
              </Field>
            ) : null}
            {draft.mode === 'image' ? (
              <Field>
                <FieldLabel htmlFor="source-image">Image</FieldLabel>
                <Input
                  id="source-image"
                  className="font-mono"
                  value={draft.imageReference}
                  disabled={saving}
                  onChange={(event) => setDraft({ ...draft, imageReference: event.target.value })}
                />
              </Field>
            ) : null}
            {draft.mode === 'connected-git' || draft.mode === 'public-git' || draft.mode === 'compose' ? (
              <Field>
                <FieldLabel htmlFor="source-branch">Branch</FieldLabel>
                <Input id="source-branch" value={draft.gitBranch} disabled={saving} onChange={(event) => setDraft({ ...draft, gitBranch: event.target.value })} />
              </Field>
            ) : null}
            {draft.mode === 'connected-git' || draft.mode === 'public-git' ? (
              <Field>
                <FieldLabel htmlFor="source-dockerfile">Dockerfile</FieldLabel>
                <Input
                  id="source-dockerfile"
                  className="font-mono"
                  value={draft.dockerfilePath}
                  disabled={saving}
                  onChange={(event) => setDraft({ ...draft, dockerfilePath: event.target.value })}
                />
              </Field>
            ) : null}
            {draft.mode === 'connected-git' || draft.mode === 'public-git' || draft.mode === 'compose' ? (
              <Field>
                <FieldLabel htmlFor="source-context">Build context</FieldLabel>
                <Input
                  id="source-context"
                  className="font-mono"
                  value={draft.buildContext}
                  disabled={saving}
                  onChange={(event) => setDraft({ ...draft, buildContext: event.target.value })}
                />
              </Field>
            ) : null}
            {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" disabled={saving} onClick={() => void save()}>
                {saving ? 'Saving…' : 'Save'}
              </Button>
              <Button type="button" size="sm" variant="outline" disabled={saving} onClick={cancelEdit}>
                Cancel
              </Button>
            </div>
          </FieldGroup>
        ) : (
          <DetailList columns={2} items={sourceRows.map((row) => ({ label: row.label, value: <ApplicationSourceValue row={row} /> }))} />
        )}
      </CardContent>
    </Card>
  )
}

function connectionInput(wire: WireGitConnection) {
  const mapped = mapWireGitConnection(wire)
  return wizardConnectionInputFromMapped({
    id: mapped.id,
    organizationId: mapped.organizationId,
    type: mapped.type,
    provider: wire.provider,
    authMode: mapped.authMode,
    providerStatus: mapped.providerStatus,
    account: mapped.account,
  })
}

function repositoryInput(wire: WireGitRepository) {
  return {
    id: wire.id,
    connectionId: wire.connectionId,
    organizationId: wire.organizationId,
    fullName: wire.fullName,
    defaultBranch: wire.defaultBranch,
    cloneUrl: wire.cloneUrl,
    metadata: wire.metadata,
  }
}

function snapshotOf(application: ApplicationDetail): ApplicationSourceSnapshot {
  return {
    sourceType: application.sourceType,
    repositoryUrl: application.repositoryUrl,
    gitBranch: application.gitBranch,
    dockerfilePath: application.dockerfilePath,
    buildContext: application.buildContext,
    imageReference: application.imageReference,
    gitConnectionId: application.gitConnectionId,
    internalPort: application.internalPort,
    command: application.command,
    entrypoint: application.entrypoint,
    cpuLimitMillis: application.cpuLimitMillis,
    memoryLimitBytes: application.memoryLimitBytes,
    restartPolicy: application.restartPolicy,
    healthCheck: application.healthCheck,
    runtimeConfig: application.runtimeConfig,
  }
}
