import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PlatformVersionsList } from '@/components/deploycore/super-admin/platform-versions-list'
import { platformVersions } from '@/lib/mock-data'

export default function AdminVersionsPage() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Component versions</CardTitle>
        <CardDescription>Deployed versions of core platform infrastructure.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <PlatformVersionsList versions={platformVersions} />
      </CardContent>
    </Card>
  )
}
