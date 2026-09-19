import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { ImagesTable } from '@/components/deploycore/images/images-table'
import { containerImages } from '@/lib/mock-data'

export default function ImagesPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Images"
        description="Built container images across connected registries. Deleting an image requires confirmation."
      />
      <Card>
        <CardContent className="p-0">
          <ImagesTable images={containerImages} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
