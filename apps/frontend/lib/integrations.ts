import type { GitProviderType, RegistryType } from '@/lib/types'

export const GIT_PROVIDER_TYPES: GitProviderType[] = [
  'GitHub',
  'GitLab',
  'Bitbucket',
  'Generic Git',
]

export const REGISTRY_TYPE_LABELS: Record<RegistryType, string> = {
  'GitHub Container Registry': 'GHCR',
  'Docker Hub': 'Docker Hub',
  'AWS ECR': 'ECR',
  'GCP Artifact Registry': 'GCP Artifact Registry',
  'Azure Container Registry': 'ACR',
  'Generic OCI Registry': 'Generic OCI',
}

export const WEBHOOK_EVENT_OPTIONS = [
  'deployment.started',
  'deployment.succeeded',
  'deployment.failed',
  'server.offline',
  'certificate.expiring',
  'backup.completed',
  'backup.failed',
] as const

export function isIntegrationConnected(status: string): boolean {
  return status !== 'unknown'
}
