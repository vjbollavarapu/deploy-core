import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { RegistriesTable } from '@/components/deploycore/registries/registries-table'
import { REGISTRY_TYPE_LABELS } from '@/lib/integrations'
import { registries } from '@/lib/mock-data'

export default function RegistriesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Registries"
        description="Container registries for GHCR, Docker Hub, GCP Artifact Registry, ECR, ACR, and generic OCI."
      />
      <Card>
        <CardHeader>
          <CardTitle>Image registries</CardTitle>
          <CardDescription>
            Supported types: {Object.values(REGISTRY_TYPE_LABELS).join(', ')}.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <RegistriesTable registries={registries} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
