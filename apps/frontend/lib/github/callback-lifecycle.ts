import {
  planGitHubCallback,
  settleGitHubCallback,
  type GitHubCallbackOutcome,
} from './callback'
import type {
  CompleteGitHubInstallationRequest,
  CompleteGitHubInstallationResponse,
} from './git-connection'

type CompleteFn = (
  body: CompleteGitHubInstallationRequest,
) => Promise<CompleteGitHubInstallationResponse>

type ActiveCallback = {
  identity: string | null
  promise: Promise<GitHubCallbackOutcome>
}

let active: ActiveCallback | null = null
let epoch = 0
let releaseQueued = false

/**
 * Identity of a completable callback. Parameter order does not matter.
 * Null means the query cannot be submitted.
 */
export function canonicalCallbackIdentity(search: string): string | null {
  const plan = planGitHubCallback(search)
  if (plan.action !== 'complete') return null
  return `${plan.body.installationId}\u0000${plan.body.setupAction}\u0000${plan.body.state}`
}

/**
 * Start or reuse the completion for this callback lifecycle.
 * An empty search reuses the in-flight callback only while that lifecycle is still active.
 */
export function beginGitHubCallback(search: string, complete: CompleteFn): Promise<GitHubCallbackOutcome> {
  retainGitHubCallbackLifecycle()
  const normalized = search.startsWith('?') ? search.slice(1) : search
  const identity = canonicalCallbackIdentity(normalized)

  if (active) {
    if (!normalized) return active.promise
    if (identity && identity === active.identity) return active.promise
  }

  const promise = settleGitHubCallback(normalized, complete)
  active = { identity, promise }
  return promise
}

/**
 * Schedule release of the in-flight callback when the effect cleans up.
 * An immediate remount calls beginGitHubCallback, which cancels the release.
 */
export function pauseGitHubCallbackLifecycle(): void {
  const epochAtPause = epoch
  releaseQueued = true
  queueMicrotask(() => {
    if (epoch !== epochAtPause || !releaseQueued) return
    active = null
    releaseQueued = false
  })
}

export function retainGitHubCallbackLifecycle(): void {
  releaseQueued = false
}

/** Test-only. Drops the in-flight callback and invalidates a queued release. */
export function resetGitHubCallbackLifecycleForTests(): void {
  epoch += 1
  active = null
  releaseQueued = false
}
