import { NetworksPageClient } from '@/components/deploycore/networks/networks-page-client'
import { networks } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function NetworksPage() {
  return <NetworksPageClient networks={getDemoFixtures(networks)} />
}

