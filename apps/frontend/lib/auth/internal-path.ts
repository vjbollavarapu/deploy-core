/**
 * Internal return paths for login. Rejects absolute and protocol-relative URLs.
 * The callback query is preserved only inside a same-origin path; it is not stored.
 */

const CONTROL_CHARS = /[\u0000-\u001F\u007F]/

export function safeInternalPath(candidate: string | null | undefined): string | null {
  if (candidate == null) return null
  const value = candidate.trim()
  if (!value || !isSafePath(value)) return null

  let decoded = value
  for (let i = 0; i < 2; i += 1) {
    let next: string
    try {
      next = decodeURIComponent(decoded)
    } catch {
      return null
    }
    if (!isSafePath(next)) return null
    if (next === decoded) break
    decoded = next
  }
  return value
}

function isSafePath(value: string): boolean {
  if (!value.startsWith('/')) return false
  if (value.startsWith('//') || value.startsWith('/\\')) return false
  if (value.includes('\\') || value.includes('://')) return false
  if (CONTROL_CHARS.test(value)) return false
  return true
}

/** Path stored in `next`, or /dashboard when the value is missing or external. */
export function pathAfterLogin(next: string | null | undefined): string {
  return safeInternalPath(next) ?? '/dashboard'
}

/**
 * Login URL for an unauthenticated visit.
 * Root `/` stays `/login`. Every other internal path keeps pathname and search.
 */
export function loginHrefForUnauthenticated(pathname: string | null, search: string): string {
  if (!pathname || pathname === '/') return '/login'
  const query = search.startsWith('?') ? search.slice(1) : search
  const target = query ? `${pathname}?${query}` : pathname
  const safe = safeInternalPath(target)
  if (!safe) return '/login'
  return `/login?next=${encodeURIComponent(safe)}`
}
