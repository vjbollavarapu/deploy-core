import { ImagesPageClient } from '@/components/deploycore/images/images-page-client'
import { containerImages } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function ImagesPage() {
  return <ImagesPageClient images={getDemoFixtures(containerImages)} />
}

