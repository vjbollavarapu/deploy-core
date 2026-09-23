'use client'

import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ApiError } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'

function slugify(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
}

/**
 * Truthful empty state when the authenticated user has zero organization memberships.
 * Registration does not create an organization; operators must create one via POST /organizations.
 */
export function CreateOrganizationEmptyState() {
  const { createOrganization } = useOrganization()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const derivedSlug = useMemo(() => (slugTouched ? slug : slugify(name)), [name, slug, slugTouched])

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const nextSlug = derivedSlug
    if (!name.trim() || !nextSlug) {
      setError('Name and slug are required.')
      return
    }
    setSubmitting(true)
    try {
      await createOrganization(name.trim(), nextSlug)
      toast.success('Organization created')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not create organization.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-[60vh] items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>Create your organization</CardTitle>
          <CardDescription>
            Your account is authenticated, but it is not a member of any organization yet.
            Registration does not create an organization automatically — create one to continue.
          </CardDescription>
        </CardHeader>
        <form onSubmit={onSubmit}>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="org-name">Organization name</FieldLabel>
                <Input
                  id="org-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Acme Platform"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="org-slug">Slug</FieldLabel>
                <Input
                  id="org-slug"
                  required
                  value={derivedSlug}
                  onChange={(e) => {
                    setSlugTouched(true)
                    setSlug(slugify(e.target.value))
                  }}
                  placeholder="acme-platform"
                />
              </Field>
              {error ? <FieldError>{error}</FieldError> : null}
            </FieldGroup>
          </CardContent>
          <CardFooter>
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Creating…' : 'Create organization'}
            </Button>
          </CardFooter>
        </form>
      </Card>
    </div>
  )
}
