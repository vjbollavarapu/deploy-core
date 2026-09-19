import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { ApplicationsFilterTable } from '@/components/deploycore/applications/applications-filter-table'
import { CreateApplicationWizard } from '@/components/deploycore/applications/create-application-wizard'
import { applications } from '@/lib/mock-data'

export default function ApplicationsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Applications"
        description="Workloads running across projects, environments, and servers."
        actions={<CreateApplicationWizard />}
      />
      <ApplicationsFilterTable applications={applications} />
    </PageContainer>
  )
}
