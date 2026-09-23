'use client'

import { ImagesFilterTable } from '@/components/deploycore/images/images-filter-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import type { ContainerImage } from '@/lib/types'

interface ImagesPageClientProps {
  images: ContainerImage[]
}

export function ImagesPageClient({ images }: ImagesPageClientProps) {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Images"
        description="Built container images across connected registries. Deleting an image requires confirmation."
      />
      <ImagesFilterTable images={images} />
    </PageContainer>
  )
}
