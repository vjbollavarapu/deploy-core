import { ApplicationsPageClient } from '@/components/deploycore/applications/applications-page-client'
import { applications } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function ApplicationsPage() {
  return <ApplicationsPageClient applications={getDemoFixtures(applications)} />
}

