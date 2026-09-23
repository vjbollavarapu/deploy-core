import { Activity } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ActivityTimeline } from '@/components/platform/activity-timeline'
import { EmptyState } from '@/components/platform/empty-state'
import { getDashboardPanels } from '@/lib/dashboard'

export function ActivityCard() {
  const { activity } = getDashboardPanels()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle>Activity Feed</CardTitle>
      </CardHeader>
      <CardContent className={activity.length === 0 ? 'py-4' : undefined}>
        {activity.length === 0 ? (
          <EmptyState
            icon={Activity}
            title="No recent activity"
            description="Activity from deployments, configuration changes, and incidents will appear here."
            className="border-0 py-2"
          />
        ) : (
          <ActivityTimeline items={activity} />
        )}
      </CardContent>
    </Card>
  )
}
