import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LogsExplorer } from '@/components/deploycore/observability/logs-explorer'

export default function LogsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Logs"
        description="Global application, server, and database logs with search, severity, and follow controls."
      />
      <LogsExplorer />
    </PageContainer>
  )
}
