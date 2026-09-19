import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { GitProvidersTable } from '@/components/deploycore/git-providers/git-providers-table'
import { GIT_PROVIDER_TYPES } from '@/lib/integrations'
import { gitProviders } from '@/lib/mock-data'

export default function GitProvidersPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Git Providers"
        description="Source control connections for GitHub, GitLab, Bitbucket, and generic Git remotes."
      />
      <Card>
        <CardHeader>
          <CardTitle>Connected providers</CardTitle>
          <CardDescription>
            Supported types: {GIT_PROVIDER_TYPES.join(', ')}.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <GitProvidersTable providers={gitProviders} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
