export interface PlacementEnvironmentOption {
  id: string
  name: string
  kind?: string
}

export interface PlacementProjectOption {
  id: string
  name: string
  environments: PlacementEnvironmentOption[]
}

export interface PlacementServerOption {
  id: string
  name: string
  region?: string
  status?: string
}

export function environmentOptionLabel(environment: PlacementEnvironmentOption): string {
  return environment.kind ? `${environment.name} (${environment.kind})` : environment.name
}

export function serverOptionLabel(server: PlacementServerOption): string {
  return server.region ? `${server.name} · ${server.region}` : server.name
}

export function isSelectableServer(server: PlacementServerOption): boolean {
  return server.status !== 'offline' && server.status !== 'OFFLINE'
}

/** Value-to-label map for Base UI Select. Keys stay the submitted ids. */
export function selectItems<T extends { id: string }>(
  rows: T[],
  label: (row: T) => string,
): Record<string, string> {
  const items: Record<string, string> = {}
  for (const row of rows) {
    if (!row.id) continue
    items[row.id] = label(row)
  }
  return items
}

export function projectSelectItems(projects: PlacementProjectOption[]): Record<string, string> {
  return selectItems(projects, (project) => project.name)
}

export function environmentSelectItems(
  projects: PlacementProjectOption[],
  projectId: string,
): Record<string, string> {
  const project = projects.find((candidate) => candidate.id === projectId)
  return selectItems(project?.environments ?? [], environmentOptionLabel)
}

export function serverSelectItems(servers: PlacementServerOption[]): Record<string, string> {
  return selectItems(servers.filter(isSelectableServer), serverOptionLabel)
}

export function placementAfterProjectChange(serverId: string): {
  environment: string
  serverId: string
} {
  return {
    environment: '',
    serverId,
  }
}

export const PLACEMENT_ORGANIZATION_MISMATCH =
  'Select a project, environment, and server for this organization.'

/** True only when placement was loaded for the active organization and the selection is in that list. */
export function placementReadyForOrganization(input: {
  activeOrganizationId: string
  loadedOrganizationId: string
  projectId: string
  environment: string
  serverId: string
  projects: readonly PlacementProjectOption[]
  servers: readonly PlacementServerOption[]
}): boolean {
  if (!input.activeOrganizationId || input.loadedOrganizationId !== input.activeOrganizationId) return false
  const project = input.projects.find((item) => item.id === input.projectId)
  if (!project) return false
  const environment = project.environments.some(
    (item) => item.id === input.environment || item.name === input.environment,
  )
  if (!environment) return false
  if (!input.serverId) return true
  return input.servers.some((item) => item.id === input.serverId)
}
