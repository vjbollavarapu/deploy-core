import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { SecretsManager } from '@/components/deploycore/secrets/secrets-manager'
import { secrets as rawSecrets } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const secrets = getDemoFixtures(rawSecrets)

export default function SecretsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Secrets"
        description="Encrypted credentials with metadata-only listings, rotation, and scoped access. Values are never returned to the UI."
      />
      <SecretsManager initialSecrets={secrets} />
    </PageContainer>
  )
}
