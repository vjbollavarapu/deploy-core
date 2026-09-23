import { ServersPageClient } from '@/components/deploycore/servers/servers-page-client'
import { servers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function ServersPage() {
  return <ServersPageClient servers={getDemoFixtures(servers)} />
}

