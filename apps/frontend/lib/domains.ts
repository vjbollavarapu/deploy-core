import type { DomainRecord, DomainTlsState, StatusTone } from '@/lib/types'
import { TONE_CLASSES } from '@/lib/status'

export const DOMAIN_TLS_STATES: DomainTlsState[] = [
  'PENDING',
  'VERIFYING',
  'ISSUING',
  'ACTIVE',
  'EXPIRING',
  'FAILED',
]

export const DOMAIN_TLS_LABELS: Record<DomainTlsState, string> = {
  PENDING: 'Pending',
  VERIFYING: 'Verifying',
  ISSUING: 'Issuing',
  ACTIVE: 'Active',
  EXPIRING: 'Expiring',
  FAILED: 'Failed',
}

export const DOMAIN_TLS_TONES: Record<DomainTlsState, StatusTone> = {
  PENDING: 'warning',
  VERIFYING: 'info',
  ISSUING: 'info',
  ACTIVE: 'success',
  EXPIRING: 'warning',
  FAILED: 'critical',
}

/** Ordered happy-path phases for the lifecycle visualiser. */
export const DOMAIN_LIFECYCLE_PHASES: DomainTlsState[] = [
  'PENDING',
  'VERIFYING',
  'ISSUING',
  'ACTIVE',
]

export function findDomain(domainId: string, domains: DomainRecord[]): DomainRecord | undefined {
  return domains.find((domain) => domain.id === domainId)
}

export function tlsToneClasses(state: DomainTlsState) {
  return TONE_CLASSES[DOMAIN_TLS_TONES[state]]
}

export function dnsMatches(domain: DomainRecord): boolean {
  if (!domain.detectedRecord) return false
  return (
    domain.detectedRecord.type === domain.requiredRecord.type &&
    domain.detectedRecord.name === domain.requiredRecord.name &&
    domain.detectedRecord.value === domain.requiredRecord.value
  )
}

export type LifecycleStepStatus = 'complete' | 'active' | 'pending' | 'failed'

export function buildDomainLifecycleSteps(
  state: DomainTlsState,
): { phase: DomainTlsState; label: string; status: LifecycleStepStatus }[] {
  const labels = DOMAIN_TLS_LABELS

  if (state === 'FAILED') {
    return DOMAIN_LIFECYCLE_PHASES.map((phase, index) => {
      // Fail during issuing by default for visualisation
      const failAt = 2
      return {
        phase,
        label: labels[phase],
        status:
          index < failAt ? 'complete' : index === failAt ? 'failed' : 'pending',
      }
    })
  }

  if (state === 'EXPIRING') {
    const steps: { phase: DomainTlsState; label: string; status: LifecycleStepStatus }[] =
      DOMAIN_LIFECYCLE_PHASES.map((phase) => ({
        phase,
        label: phase === 'ACTIVE' ? 'Expiring' : labels[phase],
        status: 'complete',
      }))
    steps.push({
      phase: 'EXPIRING',
      label: labels.EXPIRING,
      status: 'active',
    })
    return steps
  }

  const activeIndex = DOMAIN_LIFECYCLE_PHASES.indexOf(
    state === 'ACTIVE' ? 'ACTIVE' : state,
  )

  return DOMAIN_LIFECYCLE_PHASES.map((phase, index) => ({
    phase,
    label: labels[phase],
    status:
      state === 'ACTIVE'
        ? ('complete' as const)
        : index < activeIndex
          ? ('complete' as const)
          : index === activeIndex
            ? ('active' as const)
            : ('pending' as const),
  }))
}

export function expiryToneClass(days: number, hasCert: boolean): string {
  if (!hasCert) return 'text-muted-foreground'
  if (days <= 7) return 'text-critical'
  if (days <= 21) return 'text-warning'
  return 'text-foreground'
}
