import { DeploymentsPageClient } from '@/components/deploycore/deployments/deployments-page-client'
import { deployments } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function DeploymentsPage() {
  return <DeploymentsPageClient deployments={getDemoFixtures(deployments)} />
}

