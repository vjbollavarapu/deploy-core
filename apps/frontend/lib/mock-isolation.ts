/**
 * Centralized Mock & Demo Isolation Boundary for DeployCore Frontend.
 *
 * Operating Modes:
 * 1. Normal real-Control-Plane mode (Default):
 *    - isDemoModeEnabled() returns false
 *    - Real API responses are authoritative
 *    - Empty API responses render EmptyState
 *    - API failures (network, 5xx) render ErrorState with retry
 *    - Unknown IDs return undefined / trigger notFound()
 *    - Mock/fixture data is strictly NEVER displayed
 *    - Resource synthesis for unknown IDs is strictly DISABLED
 *
 * 2. Development/demo/preview mode (Explicit opt-in):
 *    - Activated ONLY when NEXT_PUBLIC_DEMO_MODE === 'true' or NEXT_PUBLIC_ENABLE_MOCK_FALLBACK === 'true'
 *    - Allows mock fixtures and synthetic fallback helpers for offline evaluation
 */

export function isDemoModeEnabled(): boolean {
  if (typeof process !== 'undefined' && process.env) {
    return (
      process.env.NEXT_PUBLIC_DEMO_MODE === 'true' ||
      process.env.NEXT_PUBLIC_ENABLE_MOCK_FALLBACK === 'true'
    )
  }
  return false
}

/**
 * Returns mock/fixture data ONLY if demo mode is explicitly enabled.
 * In normal real-Control-Plane mode, returns an empty array.
 */
export function getDemoFixtures<T>(fixtures: T[]): T[] {
  if (isDemoModeEnabled()) {
    return fixtures
  }
  return []
}

/**
 * Returns a mock/fixture record ONLY if demo mode is explicitly enabled.
 * In normal real-Control-Plane mode, returns undefined.
 */
export function getDemoFixtureItem<T>(fixture: T): T | undefined {
  if (isDemoModeEnabled()) {
    return fixture
  }
  return undefined
}

/**
 * Determines whether synthetic resource generation is permitted.
 * In normal real-Control-Plane mode, always returns false.
 */
export function allowSyntheticFallback(): boolean {
  return isDemoModeEnabled()
}
