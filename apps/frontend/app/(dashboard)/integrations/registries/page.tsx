'use client'

import { useCallback, useState } from 'react'
import { Container, Plus } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { EmptyState } from '@/components/platform/empty-state'
import { RegistriesTable } from '@/components/deploycore/registries/registries-table'
import { AddRegistryDialog } from '@/components/deploycore/registries/add-registry-dialog'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import {
  fetchRegistries,
  REGISTRY_TYPE_LABELS,
  mapWireRegistry,
} from '@/lib/integrations'
import { registries as rawRegistries } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { Registry } from '@/lib/types'

export default function RegistriesPage() {
  const { activeOrg } = useOrganization()
  const [addOpen, setAddOpen] = useState(false)

  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  const loadRegistries = useCallback(async (): Promise<Registry[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawRegistries) : []
    try {
      const res = await fetchRegistries(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map((r) => mapWireRegistry(r))
      }
      return isDemo ? getDemoFixtures(rawRegistries) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawRegistries)
      throw err
    }
  }, [orgId, isDemo])

  const { data, isLoading, error, reload } = useApiQuery(loadRegistries)
  const registries = data || []

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Registries"
        description="Container registries for GHCR, Docker Hub, GCP Artifact Registry, ECR, ACR, and generic OCI."
        actions={
          <Button
            size="sm"
            className="gap-1.5"
            onClick={() => setAddOpen(true)}
          >
            <Plus className="size-4" />
            Add Registry
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Image Registries</CardTitle>
          <CardDescription>
            Supported providers: {Object.values(REGISTRY_TYPE_LABELS).join(', ')}. Credentials are sealed with AES-256-GCM.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="py-12">
              <LoadingState label="Loading container registries…" />
            </div>
          ) : error && !isDemo ? (
            <div className="p-6">
              <ErrorState
                title="Failed to load container registries"
                message={error}
                onRetry={reload}
              />
            </div>
          ) : registries.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={Container}
                title="No registries configured"
                description="Add GHCR, Docker Hub, AWS ECR, GCP Artifact Registry, Azure ACR, or a generic OCI registry to pull private container images."
                action={
                  <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1.5">
                    <Plus className="size-4" />
                    Add Registry
                  </Button>
                }
              />
            </div>
          ) : (
            <RegistriesTable
              registries={registries}
              onRegistryChange={reload}
            />
          )}
        </CardContent>
      </Card>

      <AddRegistryDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        organizationId={orgId}
        onSuccess={reload}
      />
    </PageContainer>
  )
}
