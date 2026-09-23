import { VolumesPageClient } from '@/components/deploycore/volumes/volumes-page-client'
import { volumes } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function VolumesPage() {
  return <VolumesPageClient volumes={getDemoFixtures(volumes)} />
}

