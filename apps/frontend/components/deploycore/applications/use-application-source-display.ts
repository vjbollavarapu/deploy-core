'use client'

import { useEffect, useRef, useState } from 'react'
import { useOrganization } from '@/lib/auth-context'
import { fetchGitConnections, fetchGitRepositories, mapWireGitConnection, type WireGitRepository } from '@/lib/integrations'
import {
  acceptSourceResolution,
  applicationSourceRows,
  classifyApplicationSource,
  connectedSourceMatch,
  IDLE_GIT_METADATA,
  type ApplicationSourceFacts,
  type ConnectedGitDisplayMetadata,
  type SourceDisplayRow,
  type SourceResolutionScope,
} from '@/lib/applications/application-source-display'
import { collectGitConnectionPages } from '@/lib/applications/git-source'
import { collectRepositoryPages } from '@/lib/github/providers'
import type { ApplicationDetail } from '@/lib/control-plane/detail-read'

export function useApplicationSourceRows(
  application: ApplicationDetail | null,
  variant: 'summary' | 'detail',
): SourceDisplayRow[] {
  const { activeOrg } = useOrganization()
  const metadata = useConnectedGitMetadata(application, activeOrg?.id ?? null)
  if (!application) return []
  return applicationSourceRows(sourceFacts(application), metadata, variant)
}

function useConnectedGitMetadata(
  application: ApplicationDetail | null,
  organizationId: string | null,
): ConnectedGitDisplayMetadata {
  const [loaded, setLoaded] = useState<{
    scope: SourceResolutionScope
    metadata: ConnectedGitDisplayMetadata
  } | null>(null)
  const currentScope = useRef<SourceResolutionScope | null>(null)

  const applicationId = application?.id ?? ''
  const gitConnectionId = application?.gitConnectionId ?? null
  const repositoryUrl = application?.repositoryUrl ?? null
  const sourceType = application?.sourceType ?? null
  const connected =
    classifyApplicationSource({
      sourceType,
      repositoryUrl,
      gitBranch: application?.gitBranch ?? null,
      dockerfilePath: application?.dockerfilePath ?? null,
      buildContext: application?.buildContext ?? null,
      imageReference: application?.imageReference ?? null,
      gitConnectionId,
    }) === 'connected-git'

  useEffect(() => {
    if (!connected || !gitConnectionId || !organizationId) {
      currentScope.current = null
      return
    }
    const scope: SourceResolutionScope = {
      organizationId,
      applicationId,
      gitConnectionId,
      repositoryUrl,
    }
    currentScope.current = scope
    let active = true
    void loadConnectedMetadata(scope)
      .then((metadata) => {
        if (!active || !currentScope.current || !acceptSourceResolution(scope, currentScope.current)) return
        setLoaded({ scope, metadata })
      })
      .catch(() => {
        if (!active || !currentScope.current || !acceptSourceResolution(scope, currentScope.current)) return
        setLoaded({
          scope,
          metadata: { status: 'unavailable', connectionLabel: null, repositoryLabel: null },
        })
      })
    return () => {
      active = false
    }
  }, [applicationId, connected, gitConnectionId, organizationId, repositoryUrl])

  if (!connected || !gitConnectionId) return IDLE_GIT_METADATA
  if (!organizationId) return { status: 'unavailable', connectionLabel: null, repositoryLabel: null }
  const scope: SourceResolutionScope = {
    organizationId,
    applicationId,
    gitConnectionId,
    repositoryUrl,
  }
  if (!loaded || !acceptSourceResolution(loaded.scope, scope)) {
    return { status: 'loading', connectionLabel: null, repositoryLabel: null }
  }
  return loaded.metadata
}

async function loadConnectedMetadata(scope: SourceResolutionScope): Promise<ConnectedGitDisplayMetadata> {
  const wires = await collectGitConnectionPages(scope.organizationId, (page) => fetchGitConnections(scope.organizationId, page))
  const connections = wires
    .map((wire) => {
      const mapped = mapWireGitConnection(wire)
      return {
        id: mapped.id,
        organizationId: mapped.organizationId,
        provider: wire.provider,
        type: mapped.type,
        authMode: mapped.authMode,
        providerStatus: mapped.providerStatus ?? wire.status,
        account: mapped.account,
      }
    })
    .filter((connection) => connection.organizationId === scope.organizationId && connection.id === scope.gitConnectionId)
  const connection = connections[0]
  if (!connection) {
    return { status: 'unavailable', connectionLabel: null, repositoryLabel: null }
  }
  let repositories: WireGitRepository[] = []
  try {
    repositories = await collectRepositoryPages((page) => fetchGitRepositories(connection.id, page))
  } catch {
    const partial = connectedSourceMatch({
      organizationId: scope.organizationId,
      gitConnectionId: scope.gitConnectionId,
      repositoryUrl: scope.repositoryUrl,
      connections,
      repositories: [],
    })
    return { status: 'unavailable', connectionLabel: partial.connectionLabel, repositoryLabel: null }
  }
  const match = connectedSourceMatch({
    organizationId: scope.organizationId,
    gitConnectionId: scope.gitConnectionId,
    repositoryUrl: scope.repositoryUrl,
    connections,
    repositories: repositories
      .filter((repository) => repository.organizationId === scope.organizationId && repository.connectionId === connection.id)
      .map((repository) => ({
        id: repository.id,
        connectionId: repository.connectionId,
        organizationId: repository.organizationId,
        fullName: repository.fullName,
        cloneUrl: repository.cloneUrl,
      })),
  })
  return { status: 'resolved', connectionLabel: match.connectionLabel, repositoryLabel: match.repositoryLabel }
}

function sourceFacts(application: ApplicationDetail): ApplicationSourceFacts {
  return {
    sourceType: application.sourceType,
    repositoryUrl: application.repositoryUrl,
    gitBranch: application.gitBranch,
    dockerfilePath: application.dockerfilePath,
    buildContext: application.buildContext,
    imageReference: application.imageReference,
    gitConnectionId: application.gitConnectionId,
  }
}
