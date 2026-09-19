import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { EnvironmentVariablesEditor } from '@/components/deploycore/environment-variables/environment-variables-editor'
import { envVarHierarchy } from '@/lib/mock-data'

export default function EnvironmentVariablesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Environment Variables"
        description="Configuration resolved through Organization → Project → Environment → Application. Show inherited, overridden, and application-specific values."
      />
      <Card>
        <CardContent className="p-0">
          <EnvironmentVariablesEditor initialVariables={envVarHierarchy} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
