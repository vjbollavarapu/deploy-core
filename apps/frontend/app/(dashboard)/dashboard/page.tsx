import Link from 'next/link'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { ActivityCard } from '@/components/deploycore/dashboard/activity-card'
import { AppsRequiringAttention } from '@/components/deploycore/dashboard/apps-requiring-attention'
import { BackupStatusCard } from '@/components/deploycore/dashboard/backup-status-card'
import { CertificateWarningsCard } from '@/components/deploycore/dashboard/certificate-warnings-card'
import { InfrastructureStatus } from '@/components/deploycore/dashboard/infrastructure-status'
import { RecentDeploymentsCard } from '@/components/deploycore/dashboard/recent-deployments-card'
import { RecentIncidentsCard } from '@/components/deploycore/dashboard/recent-incidents-card'
import { ResourceUtilisation } from '@/components/deploycore/dashboard/resource-utilisation'
import { ServerCapacity } from '@/components/deploycore/dashboard/server-capacity'

export default function DashboardPage() {
  return (
    <PageContainer density="wide" className="gap-8">
      <PageHeader
        title="Overview"
        description="Fleet health, capacity, and operational signals across the control plane."
        actions={
          <Button size="sm" nativeButton={false} render={<Link href="/deployments" />}>
            <Plus data-icon="inline-start" />
            New Deployment
          </Button>
        }
      />

      <InfrastructureStatus />
      <ResourceUtilisation />

      <div className="grid gap-4 xl:grid-cols-3">
        <div className="flex flex-col gap-4 xl:col-span-2">
          <RecentDeploymentsCard />
          <AppsRequiringAttention />
          <ServerCapacity />
        </div>
        <div className="flex flex-col gap-4">
          <RecentIncidentsCard />
          <CertificateWarningsCard />
          <BackupStatusCard />
          <ActivityCard />
        </div>
      </div>
    </PageContainer>
  )
}
