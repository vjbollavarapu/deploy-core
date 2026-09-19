import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { SecretsManager } from '@/components/deploycore/secrets/secrets-manager'
import { secrets } from '@/lib/mock-data'

export default function SecretsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Secrets"
        description="Encrypted credentials with metadata-only listings, rotation, and scoped access. Values are never returned to the UI."
      />
      <Card>
        <CardContent className="p-0">
          <SecretsManager initialSecrets={secrets} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
