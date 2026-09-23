import { ContainersPageClient } from '@/components/deploycore/containers/containers-page-client'
import { containers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function ContainersPage() {
  return <ContainersPageClient containers={getDemoFixtures(containers)} />
}

