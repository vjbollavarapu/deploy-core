import { notFound } from 'next/navigation'
import { ApplicationOverview } from '@/components/deploycore/applications/application-overview'
import {
  findApplication,
  getApplicationActivity,
  getApplicationLogs,
  getLatestDeployment,
  getPrimaryDomain,
} from '@/lib/applications'

export default async function ApplicationOverviewPage({
  params,
}: {
  params: Promise<{ applicationId: string }>
}) {
  const { applicationId } = await params
  const application = findApplication(applicationId)
  if (!application) notFound()

  return (
    <ApplicationOverview
      application={application}
      latestDeployment={getLatestDeployment(application)}
      primaryDomain={getPrimaryDomain(application)}
      recentLogs={getApplicationLogs(application, 24)}
      activity={getApplicationActivity(application)}
    />
  )
}
