import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ActivityTimeline } from '@/components/platform/activity-timeline'
import { getDashboardPanels } from '@/lib/dashboard'

export function ActivityCard() {
  const { activity } = getDashboardPanels()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle>Activity Feed</CardTitle>
      </CardHeader>
      <CardContent>
        <ActivityTimeline items={activity} />
      </CardContent>
    </Card>
  )
}
