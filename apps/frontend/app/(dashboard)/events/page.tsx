import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { EventsFeed } from '@/components/deploycore/observability/events-feed'

export default function EventsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Events"
        description="Platform and infrastructure events from deployments, backups, certificates, and incidents."
      />
      <EventsFeed />
    </PageContainer>
  )
}
