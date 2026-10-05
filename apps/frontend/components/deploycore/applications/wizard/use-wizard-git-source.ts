'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import type { Control, UseFormGetValues, UseFormSetValue } from 'react-hook-form'
import { useWatch } from 'react-hook-form'
import { mapWireGitConnection, fetchGitConnections, fetchGitRepositories } from '@/lib/integrations'
import {
  applyConnectedGitTransition,
  applyConnectionChange,
  applyOrganizationChange,
  applyPublicGitTransition,
  applyRepositorySelection,
  applySourceTypeChange,
  connectionFromOption,
  connectionLoadApplies,
  connectionSelectionAfterRefresh,
  commitWizardGitCache,
  connectionSelectionAllowed,
  defaultRepositorySource,
  emptyWizardGitCache,
  loadWizardGitConnections,
  loadWizardRepositories,
  readGitForm,
  repositoryLoadResult,
  visibleWizardGitCache,
  wizardConnectionInputFromMapped,
  wizardConnectionOptions,
  wizardGitCacheAfterConnectionFailure,
  wizardGitCacheAfterConnectionLoading,
  wizardGitCacheAfterConnectionsLoaded,
  wizardGitCacheAfterOrganizationChange,
  wizardGitCacheAfterRepositoriesLoaded,
  wizardRepositoryOptions,
  type WizardGitCache,
  type WizardGitFormState,
} from '@/lib/applications/git-source-wizard'
import type { CreateApplicationValues, RepositorySourceMode, SourceType } from '@/lib/validations/application'

function writeGitForm(setValue: UseFormSetValue<CreateApplicationValues>, state: WizardGitFormState) {
  setValue('repositorySource', state.repositorySource)
  setValue('gitConnectionId', state.gitConnectionId)
  setValue('repositoryId', state.repositoryId)
  setValue('repository', state.repository)
  setValue('branch', state.branch)
  setValue('projectId', state.projectId)
  setValue('environment', state.environment)
  setValue('serverId', state.serverId)
}

export function useWizardGitSource(input: {
  open: boolean
  organizationId: string
  control: Control<CreateApplicationValues>
  getValues: UseFormGetValues<CreateApplicationValues>
  setValue: UseFormSetValue<CreateApplicationValues>
}) {
  const { open, organizationId, control, getValues, setValue } = input
  const sourceType = useWatch({ control, name: 'sourceType' })
  const repositorySource = useWatch({ control, name: 'repositorySource' })
  const gitConnectionId = useWatch({ control, name: 'gitConnectionId' })

  const [cache, setCache] = useState<WizardGitCache>(emptyWizardGitCache)
  const visible = useMemo(() => visibleWizardGitCache(cache, organizationId), [cache, organizationId])

  const modeChosen = useRef(false)
  const connectionGeneration = useRef(0)
  const repositoryGeneration = useRef(0)
  const openRef = useRef(open)
  const organizationRef = useRef(organizationId)
  const previousOrganization = useRef<string | null>(null)

  useEffect(() => {
    openRef.current = open
    organizationRef.current = organizationId
  }, [open, organizationId])

  useEffect(() => {
    if (!open) modeChosen.current = false
  }, [open])

  useEffect(() => {
    const previous = previousOrganization.current
    if (!previous) {
      previousOrganization.current = organizationId
      return
    }
    if (previous === organizationId) return
    previousOrganization.current = organizationId
    connectionGeneration.current += 1
    repositoryGeneration.current += 1
    let cancelled = false
    async function resetForOrganization() {
      await Promise.resolve()
      if (cancelled || organizationRef.current !== organizationId) return
      setCache((current) => wizardGitCacheAfterOrganizationChange(current, organizationId))
      if (!openRef.current) return
      writeGitForm(setValue, applyOrganizationChange(readGitForm(getValues())))
    }
    void resetForOrganization()
    return () => {
      cancelled = true
    }
  }, [organizationId, getValues, setValue])

  useEffect(() => {
    if (!open || !organizationId || sourceType !== 'git') return
    let cancelled = false
    const generation = connectionGeneration.current
    const requested = {
      open: true,
      organizationId,
      sourceType: 'git' as const,
      generation,
    }
    async function loadConnections() {
      await Promise.resolve()
      if (cancelled) return
      setCache((current) => {
        const next = wizardGitCacheAfterConnectionLoading(current, {
          activeOrganizationId: organizationRef.current,
          requestedOrganizationId: organizationId,
        })
        return commitWizardGitCache(current, organizationRef.current, next)
      })
      try {
        const wires = await loadWizardGitConnections(organizationId, async (page) => {
          const response = await fetchGitConnections(page.organizationId, {
            limit: page.limit,
            offset: page.offset,
          })
          return { items: response.items ?? [], totalCount: response.totalCount }
        })
        const current = {
          open: openRef.current,
          organizationId: organizationRef.current,
          sourceType: getValues('sourceType'),
          generation: connectionGeneration.current,
        }
        if (cancelled || !connectionLoadApplies(requested, current)) return
        const options = wizardConnectionOptions(
          wires.map((wire) => wizardConnectionInputFromMapped(mapWireGitConnection(wire))),
          organizationId,
        )
        setCache((current) => {
          const next = wizardGitCacheAfterConnectionsLoaded(current, {
            activeOrganizationId: organizationRef.current,
            requestedOrganizationId: organizationId,
            connections: options,
          })
          return commitWizardGitCache(current, organizationRef.current, next)
        })
        const loaded = readGitForm(getValues())
        if (loaded.repositorySource === 'connected') {
          const refreshed = connectionSelectionAfterRefresh(loaded, options, organizationId)
          if (
            refreshed.gitConnectionId !== loaded.gitConnectionId ||
            refreshed.repositoryId !== loaded.repositoryId ||
            refreshed.repository !== loaded.repository
          ) {
            writeGitForm(setValue, refreshed)
          }
        }
        const latest = readGitForm(getValues())
        const nextMode = defaultRepositorySource({
          selectableCount: options.filter((option) => option.selectable).length,
          repository: latest.repository,
          gitConnectionId: latest.gitConnectionId,
          repositoryId: latest.repositoryId,
          modeChosen: modeChosen.current,
          current: latest.repositorySource,
        })
        if (nextMode && nextMode !== latest.repositorySource && getValues('sourceType') === 'git') {
          setValue('repositorySource', nextMode)
        }
      } catch {
        const current = {
          open: openRef.current,
          organizationId: organizationRef.current,
          sourceType: getValues('sourceType'),
          generation: connectionGeneration.current,
        }
        if (cancelled || !connectionLoadApplies(requested, current)) return
        setCache((current) => {
          const next = wizardGitCacheAfterConnectionFailure(current, {
            activeOrganizationId: organizationRef.current,
            requestedOrganizationId: organizationId,
          })
          return commitWizardGitCache(current, organizationRef.current, next)
        })
      }
    }
    void loadConnections()
    return () => {
      cancelled = true
      connectionGeneration.current += 1
    }
  }, [open, organizationId, sourceType, getValues, setValue])

  useEffect(() => {
    const connection = visible.connections.find((option) => option.id === gitConnectionId && option.selectable)
    const shouldLoad = Boolean(
      open && organizationId && sourceType === 'git' && repositorySource === 'connected' && connection,
    )
    if (!shouldLoad || !connection) {
      let cancelled = false
      async function clearRepositories() {
        await Promise.resolve()
        if (cancelled) return
        setCache((current) => {
          const next = wizardGitCacheAfterRepositoriesLoaded(current, {
            activeOrganizationId: organizationRef.current,
            requestedOrganizationId: organizationId,
            repositories: [],
            phase: 'idle',
          })
          return commitWizardGitCache(current, organizationRef.current, next)
        })
      }
      void clearRepositories()
      return () => {
        cancelled = true
      }
    }

    const selectedConnection = connection
    let cancelled = false
    const generation = repositoryGeneration.current
    const requested = {
      open: true,
      organizationId,
      connectionId: selectedConnection.id,
      repositorySource: 'connected' as const,
      sourceType: 'git' as const,
      generation,
    }
    async function loadRepositories() {
      await Promise.resolve()
      if (cancelled || repositoryGeneration.current !== generation) return
      setCache((current) => {
        const next = wizardGitCacheAfterRepositoriesLoaded(current, {
          activeOrganizationId: organizationRef.current,
          requestedOrganizationId: organizationId,
          repositories: [],
          phase: 'loading',
        })
        return commitWizardGitCache(current, organizationRef.current, next)
      })
      try {
        const wires = await loadWizardRepositories((page) =>
          fetchGitRepositories(selectedConnection.id, page).then((response) => ({
            items: response.items ?? [],
            totalCount: response.totalCount,
          })),
        )
        const values = getValues()
        const loaded = repositoryLoadResult(requested, {
          open: openRef.current,
          organizationId: organizationRef.current,
          connectionId: values.gitConnectionId,
          repositorySource: values.repositorySource,
          sourceType: values.sourceType,
          generation: repositoryGeneration.current,
        }, wires)
        if (cancelled || !loaded) return
        const repositories = wizardRepositoryOptions({
          organizationId,
          connection: connectionFromOption(selectedConnection),
          repositories: loaded.map((wire) => ({
            id: wire.id,
            connectionId: wire.connectionId,
            organizationId: wire.organizationId,
            fullName: wire.fullName,
            defaultBranch: wire.defaultBranch,
            cloneUrl: wire.cloneUrl,
            metadata: wire.metadata
              ? {
                  private: wire.metadata.private,
                  archived: wire.metadata.archived,
                  visibility: wire.metadata.visibility,
                }
              : undefined,
          })),
        })
        setCache((current) => {
          const next = wizardGitCacheAfterRepositoriesLoaded(current, {
            activeOrganizationId: organizationRef.current,
            requestedOrganizationId: organizationId,
            repositories,
            phase: 'ready',
          })
          return commitWizardGitCache(current, organizationRef.current, next)
        })
      } catch {
        if (cancelled || repositoryGeneration.current !== generation) return
        if (!openRef.current || organizationRef.current !== organizationId) return
        if (getValues('gitConnectionId') !== selectedConnection.id || getValues('repositorySource') !== 'connected') return
        if (getValues('sourceType') !== 'git') return
        setCache((current) => {
          const next = wizardGitCacheAfterRepositoriesLoaded(current, {
            activeOrganizationId: organizationRef.current,
            requestedOrganizationId: organizationId,
            repositories: [],
            phase: 'error',
          })
          return commitWizardGitCache(current, organizationRef.current, next)
        })
      }
    }
    void loadRepositories()
    return () => {
      cancelled = true
      repositoryGeneration.current += 1
    }
  }, [open, organizationId, sourceType, repositorySource, gitConnectionId, visible.connections, getValues])

  function selectSourceType(next: SourceType) {
    const current = readGitForm(getValues())
    if (current.sourceType === next) return
    const updated = applySourceTypeChange(current, next)
    if (current.sourceType === 'git' && next !== 'git') {
      writeGitForm(setValue, updated)
      setCache((current) => {
        const nextCache = wizardGitCacheAfterRepositoriesLoaded(current, {
          activeOrganizationId: organizationId,
          requestedOrganizationId: organizationId,
          repositories: [],
          phase: 'idle',
        })
        return commitWizardGitCache(current, organizationId, nextCache)
      })
    }
    setValue('sourceType', next, { shouldValidate: true })
  }

  function selectRepositorySource(next: RepositorySourceMode) {
    modeChosen.current = true
    const current = readGitForm(getValues())
    if (current.repositorySource === next) return
    const updated = next === 'public' ? applyPublicGitTransition(current) : applyConnectedGitTransition(current)
    writeGitForm(setValue, updated)
    if (next === 'public') {
      setCache((current) => {
        const nextCache = wizardGitCacheAfterRepositoriesLoaded(current, {
          activeOrganizationId: organizationId,
          requestedOrganizationId: organizationId,
          repositories: [],
          phase: 'idle',
        })
        return commitWizardGitCache(current, organizationId, nextCache)
      })
    }
  }

  function selectConnection(connectionId: string) {
    const option = visible.connections.find((item) => item.id === connectionId)
    if (!connectionSelectionAllowed(option, organizationId) || getValues('gitConnectionId') === connectionId) return
    writeGitForm(setValue, applyConnectionChange(readGitForm(getValues()), connectionId))
    setCache((current) => {
      const nextCache = wizardGitCacheAfterRepositoriesLoaded(current, {
        activeOrganizationId: organizationId,
        requestedOrganizationId: organizationId,
        repositories: [],
        phase: 'idle',
      })
      return commitWizardGitCache(current, organizationId, nextCache)
    })
  }

  function selectRepository(repositoryId: string) {
    const option = visible.repositories.find((item) => item.id === repositoryId)
    const selectedConnection = visible.connections.find((item) => item.id === getValues('gitConnectionId'))
    if (
      !connectionSelectionAllowed(selectedConnection, organizationId) ||
      !option?.selectable ||
      !option.selection ||
      !organizationId ||
      getValues('repositoryId') === repositoryId
    ) {
      return
    }
    const result = applyRepositorySelection(readGitForm(getValues()), {
      organizationId,
      connection: connectionFromOption(selectedConnection),
      repository: option.selection,
    })
    if (!result.ok) return
    writeGitForm(setValue, result.state)
  }

  return {
    connections: visible.connections,
    repositories: visible.repositories,
    connectionPhase: visible.connectionPhase,
    repositoryPhase: visible.repositoryPhase,
    selectSourceType,
    selectRepositorySource,
    selectConnection,
    selectRepository,
  }
}
