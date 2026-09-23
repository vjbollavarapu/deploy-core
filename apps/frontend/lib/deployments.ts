import { deployments as rawDeployments } from '@/lib/mock-data'
import { getDemoFixtures, allowSyntheticFallback } from '@/lib/mock-isolation'
import type {
  Deployment,
  DeploymentEvent,
  DeploymentFailureReason,
  DeploymentPhase,
  DeploymentStep,
  Status,
  StatusTone,
} from '@/lib/types'
import { TONE_CLASSES } from '@/lib/status'

export function findDeployment(deploymentId: string, list?: Deployment[]): Deployment | undefined {
  const source = list ?? getDemoFixtures(rawDeployments)
  const found = source.find((d) => d.id === deploymentId || String(d.number) === deploymentId)
  if (found) return found
  if (allowSyntheticFallback() && deploymentId && deploymentId !== 'undefined') {
    const num = Number.parseInt(deploymentId.replace(/\D/g, ''), 10) || 101
    return {
      id: deploymentId,
      number: num,
      applicationId: 'app-ecommerce-api',
      application: 'Core Platform Service',
      project: 'Core Platform',
      environment: 'production',
      revision: `rev-${num}`,
      commit: '7f9a12c',
      commitMessage: 'Deploy workload to cluster',
      author: { name: 'Operator' },
      triggeredBy: 'Manual Trigger',
      status: 'healthy',
      phase: 'RUNNING',
      duration: '42s',
      startedAt: 'Just now',
      repo: 'github.com/deploycore/service',
      branch: 'main',
      server: 'hetzner-fsn1-01',
      image: 'ghcr.io/deploycore/app:latest',
      steps: buildDeploymentSteps('healthy'),
      events: buildDeploymentEvents('healthy', 'Core Platform Service'),
    }
  }
  return undefined
}

export const DEPLOYMENT_PHASES: DeploymentPhase[] = [
  'PENDING',
  'QUEUED',
  'PREPARING',
  'FETCHING_SOURCE',
  'BUILDING',
  'IMAGE_READY',
  'CREATING_CONTAINER',
  'STARTING',
  'HEALTH_CHECKING',
  'ACTIVATING',
  'RUNNING',
]

export const DEPLOYMENT_PHASE_LABELS: Record<DeploymentPhase, string> = {
  PENDING: 'Pending',
  QUEUED: 'Queued',
  PREPARING: 'Preparing',
  FETCHING_SOURCE: 'Fetching source',
  BUILDING: 'Building',
  IMAGE_READY: 'Image ready',
  CREATING_CONTAINER: 'Creating container',
  STARTING: 'Starting',
  HEALTH_CHECKING: 'Health checking',
  ACTIVATING: 'Activating',
  RUNNING: 'Running',
}

export const DEPLOYMENT_FAILURE_LABELS: Record<DeploymentFailureReason, string> = {
  SOURCE_FAILED: 'Source failed',
  BUILD_FAILED: 'Build failed',
  IMAGE_FAILED: 'Image failed',
  CONTAINER_FAILED: 'Container failed',
  START_FAILED: 'Start failed',
  HEALTH_CHECK_FAILED: 'Health check failed',
  ROUTING_FAILED: 'Routing failed',
  CANCELLED: 'Cancelled',
  TIMEOUT: 'Timeout',
}

export const DEPLOYMENT_FILTERS = [
  { id: 'all', label: 'All' },
  { id: 'queued', label: 'Queued' },
  { id: 'running', label: 'Running' },
  { id: 'successful', label: 'Successful' },
  { id: 'failed', label: 'Failed' },
  { id: 'cancelled', label: 'Cancelled' },
] as const

export type DeploymentFilterId = (typeof DEPLOYMENT_FILTERS)[number]['id']

export function matchesDeploymentFilter(deployment: Deployment, filter: DeploymentFilterId): boolean {
  switch (filter) {
    case 'queued':
      return deployment.status === 'queued' || deployment.status === 'pending'
    case 'running':
      return deployment.status === 'deploying' || deployment.status === 'running'
    case 'successful':
      return deployment.status === 'healthy'
    case 'failed':
      return deployment.status === 'failed'
    case 'cancelled':
      return deployment.status === 'cancelled' || deployment.status === 'stopped'
    case 'all':
    default:
      return true
  }
}

export function phaseLabel(phase: DeploymentPhase | DeploymentFailureReason): string {
  if (phase in DEPLOYMENT_PHASE_LABELS) {
    return DEPLOYMENT_PHASE_LABELS[phase as DeploymentPhase]
  }
  return DEPLOYMENT_FAILURE_LABELS[phase as DeploymentFailureReason]
}

export function failurePhaseIndex(reason: DeploymentFailureReason): number {
  switch (reason) {
    case 'SOURCE_FAILED':
      return DEPLOYMENT_PHASES.indexOf('FETCHING_SOURCE')
    case 'BUILD_FAILED':
      return DEPLOYMENT_PHASES.indexOf('BUILDING')
    case 'IMAGE_FAILED':
      return DEPLOYMENT_PHASES.indexOf('IMAGE_READY')
    case 'CONTAINER_FAILED':
      return DEPLOYMENT_PHASES.indexOf('CREATING_CONTAINER')
    case 'START_FAILED':
      return DEPLOYMENT_PHASES.indexOf('STARTING')
    case 'HEALTH_CHECK_FAILED':
      return DEPLOYMENT_PHASES.indexOf('HEALTH_CHECKING')
    case 'ROUTING_FAILED':
      return DEPLOYMENT_PHASES.indexOf('ACTIVATING')
    case 'CANCELLED':
    case 'TIMEOUT':
      return DEPLOYMENT_PHASES.indexOf('PREPARING')
  }
}

export function buildDeploymentSteps(
  status: Status,
  failureReason?: DeploymentFailureReason,
): DeploymentStep[] {
  const phases = DEPLOYMENT_PHASES

  if (status === 'healthy' || status === 'running') {
    return phases.map((phase) => ({
      phase,
      name: DEPLOYMENT_PHASE_LABELS[phase],
      status: 'complete' as const,
    }))
  }

  if (status === 'queued' || status === 'pending') {
    return phases.map((phase, index) => ({
      phase,
      name: DEPLOYMENT_PHASE_LABELS[phase],
      status: index === 0 ? ('active' as const) : index === 1 && status === 'queued' ? ('active' as const) : ('pending' as const),
    })).map((step, index) => {
      if (status === 'queued') {
        return {
          ...step,
          status: index === 0 ? 'complete' : index === 1 ? 'active' : 'pending',
        }
      }
      return {
        ...step,
        status: index === 0 ? 'active' : 'pending',
      }
    })
  }

  if (status === 'deploying') {
    const activeIndex = phases.indexOf('CREATING_CONTAINER')
    return phases.map((phase, index) => ({
      phase,
      name: DEPLOYMENT_PHASE_LABELS[phase],
      status: index < activeIndex ? 'complete' : index === activeIndex ? 'active' : 'pending',
    }))
  }

  if (status === 'failed') {
    const failIndex = failurePhaseIndex(failureReason ?? 'BUILD_FAILED')
    return phases.map((phase, index) => ({
      phase,
      name: DEPLOYMENT_PHASE_LABELS[phase],
      status: index < failIndex ? 'complete' : index === failIndex ? 'failed' : 'pending',
      failureReason: index === failIndex ? failureReason ?? 'BUILD_FAILED' : undefined,
    }))
  }

  if (status === 'cancelled' || status === 'stopped') {
    const cancelIndex = failurePhaseIndex('CANCELLED')
    return phases.map((phase, index) => ({
      phase,
      name: DEPLOYMENT_PHASE_LABELS[phase],
      status: index < cancelIndex ? 'complete' : index === cancelIndex ? 'failed' : 'pending',
      failureReason: index === cancelIndex ? 'CANCELLED' : undefined,
    }))
  }

  return phases.map((phase) => ({
    phase,
    name: DEPLOYMENT_PHASE_LABELS[phase],
    status: 'pending' as const,
  }))
}

export function buildDeploymentEvents(
  status: Status,
  application: string,
  failureReason?: DeploymentFailureReason,
): DeploymentEvent[] {
  const base: DeploymentEvent[] = [
    {
      id: 'evt-1',
      timestamp: 't+0s',
      phase: 'PENDING',
      message: `Deployment request accepted for ${application}`,
      tone: 'info',
    },
    {
      id: 'evt-2',
      timestamp: 't+1s',
      phase: 'QUEUED',
      message: 'Waiting for executor capacity',
      tone: 'warning',
    },
  ]

  if (status === 'queued' || status === 'pending') return base

  base.push(
    {
      id: 'evt-3',
      timestamp: 't+3s',
      phase: 'PREPARING',
      message: 'Allocated build workspace and credentials',
      tone: 'info',
    },
    {
      id: 'evt-4',
      timestamp: 't+8s',
      phase: 'FETCHING_SOURCE',
      message: 'Cloned repository and resolved commit',
      tone: 'info',
    },
  )

  if (status === 'failed' && failureReason === 'SOURCE_FAILED') {
    base.push({
      id: 'evt-fail',
      timestamp: 't+12s',
      phase: 'SOURCE_FAILED',
      message: 'Unable to fetch source — authentication or ref missing',
      tone: 'critical',
    })
    return base
  }

  base.push({
    id: 'evt-5',
    timestamp: 't+18s',
    phase: 'BUILDING',
    message: 'Build started',
    tone: 'info',
  })

  if (status === 'failed' && (failureReason === 'BUILD_FAILED' || !failureReason)) {
    base.push({
      id: 'evt-fail',
      timestamp: 't+38s',
      phase: 'BUILD_FAILED',
      message: 'Build exited with non-zero status',
      tone: 'critical',
    })
    return base
  }

  if (status === 'cancelled') {
    base.push({
      id: 'evt-cancel',
      timestamp: 't+22s',
      phase: 'CANCELLED',
      message: 'Deployment cancelled by operator',
      tone: 'inactive',
    })
    return base
  }

  base.push(
    {
      id: 'evt-6',
      timestamp: 't+52s',
      phase: 'IMAGE_READY',
      message: 'Image pushed to registry',
      tone: 'success',
    },
    {
      id: 'evt-7',
      timestamp: 't+58s',
      phase: 'CREATING_CONTAINER',
      message: 'Creating container on target server',
      tone: 'info',
    },
  )

  if (status === 'deploying') {
    base.push({
      id: 'evt-live',
      timestamp: 't+62s',
      phase: 'CREATING_CONTAINER',
      message: 'Pulling layers and configuring mounts',
      tone: 'info',
    })
    return base
  }

  base.push(
    {
      id: 'evt-8',
      timestamp: 't+70s',
      phase: 'STARTING',
      message: 'Container process started',
      tone: 'info',
    },
    {
      id: 'evt-9',
      timestamp: 't+82s',
      phase: 'HEALTH_CHECKING',
      message: 'Waiting for health endpoint',
      tone: 'info',
    },
    {
      id: 'evt-10',
      timestamp: 't+95s',
      phase: 'ACTIVATING',
      message: 'Routing traffic to new revision',
      tone: 'info',
    },
    {
      id: 'evt-11',
      timestamp: 't+102s',
      phase: 'RUNNING',
      message: 'Deployment active',
      tone: 'success',
    },
  )

  if (status === 'failed' && failureReason === 'HEALTH_CHECK_FAILED') {
    return [
      ...base.slice(0, -3),
      {
        id: 'evt-fail',
        timestamp: 't+90s',
        phase: 'HEALTH_CHECK_FAILED',
        message: 'Health checks failed after retries',
        tone: 'critical',
      },
    ]
  }

  return base
}

export function currentPhaseForStatus(
  status: Status,
  failureReason?: DeploymentFailureReason,
): DeploymentPhase {
  if (status === 'healthy' || status === 'running') return 'RUNNING'
  if (status === 'queued') return 'QUEUED'
  if (status === 'pending') return 'PENDING'
  if (status === 'deploying') return 'CREATING_CONTAINER'
  if (status === 'failed') {
    const index = failurePhaseIndex(failureReason ?? 'BUILD_FAILED')
    return DEPLOYMENT_PHASES[index] ?? 'BUILDING'
  }
  if (status === 'cancelled' || status === 'stopped') return 'PREPARING'
  return 'PENDING'
}

export function phaseToneClasses(tone: StatusTone) {
  return TONE_CLASSES[tone]
}
