import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { MetricsDashboard } from '@/components/deploycore/observability/metrics-dashboard'

export default function MetricsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Metrics"
        description="CPU, RAM, storage, restart count, and uptime from API entity fields. Network is shown only when provided."
      />
      <MetricsDashboard />
    </PageContainer>
  )
}
