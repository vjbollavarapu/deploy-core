import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { FeatureFlagsList } from '@/components/deploycore/super-admin/feature-flags-list'
import { featureFlags } from '@/lib/mock-data'

export default function AdminFeaturesPage() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Feature flags</CardTitle>
        <CardDescription>Control rollout of in-progress platform features.</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <FeatureFlagsList flags={featureFlags} />
      </CardContent>
    </Card>
  )
}
