import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { loginHrefForUnauthenticated, pathAfterLogin, safeInternalPath } from './internal-path'

const CALLBACK =
  '/integrations/git/github/callback?installation_id=42&setup_action=install&state=abc'

describe('safe internal return paths', () => {
  it('accepts an internal dashboard path', () => {
    assert.equal(safeInternalPath('/servers'), '/servers')
    assert.equal(pathAfterLogin('/dashboard'), '/dashboard')
  })

  it('preserves pathname and search for the GitHub callback', () => {
    assert.equal(safeInternalPath(CALLBACK), CALLBACK)
    assert.equal(
      loginHrefForUnauthenticated('/integrations/git/github/callback', '?installation_id=42&setup_action=install&state=abc'),
      `/login?next=${encodeURIComponent(CALLBACK)}`,
    )
  })

  it('keeps an ordinary dashboard login redirect', () => {
    assert.equal(loginHrefForUnauthenticated('/applications', ''), '/login?next=%2Fapplications')
    assert.equal(loginHrefForUnauthenticated('/', ''), '/login')
    assert.equal(loginHrefForUnauthenticated(null, ''), '/login')
  })

  it('rejects an absolute external next URL', () => {
    assert.equal(safeInternalPath('https://evil.example/phish'), null)
    assert.equal(pathAfterLogin('https://evil.example/phish'), '/dashboard')
  })

  it('rejects a protocol-relative next URL', () => {
    assert.equal(safeInternalPath('//evil.example/phish'), null)
    assert.equal(safeInternalPath('/%2f%2fevil.example'), null)
    assert.equal(pathAfterLogin('//evil.example'), '/dashboard')
  })

  it('rejects a javascript URL and a backslash path', () => {
    assert.equal(safeInternalPath('javascript:alert(1)'), null)
    assert.equal(pathAfterLogin('javascript:alert(1)'), '/dashboard')
    assert.equal(safeInternalPath('/\\evil.example'), null)
    assert.equal(pathAfterLogin('/\\evil.example'), '/dashboard')
  })
})
