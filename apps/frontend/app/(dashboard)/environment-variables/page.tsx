import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { EnvironmentVariablesEditor } from '@/components/deploycore/environment-variables/environment-variables-editor'
import { envVarHierarchy as rawEnvVarHierarchy } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const envVarHierarchy = getDemoFixtures(rawEnvVarHierarchy)

export default function EnvironmentVariablesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Environment Variables"
        description="Configuration resolved through Organization → Project → Environment → Application. Show inherited, overridden, and application-specific values."
      />
      <EnvironmentVariablesEditor initialVariables={envVarHierarchy} />
    </PageContainer>
  )
}
