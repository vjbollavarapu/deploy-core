import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { NotificationChannelsList } from '@/components/deploycore/notifications/notification-channels-list'
import { NotificationPoliciesList } from '@/components/deploycore/notifications/notification-policies-list'
import { notificationChannels, notificationPolicies } from '@/lib/mock-data'

export default function NotificationsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Notifications"
        description="Alert channels and policies that route deployment, server, backup, and certificate events."
      />

      <Card>
        <CardHeader>
          <CardTitle>Channels</CardTitle>
          <CardDescription>
            Email, Slack, Microsoft Teams, Discord, Telegram, Webhook, and WhatsApp destinations.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <NotificationChannelsList channels={notificationChannels} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Policies</CardTitle>
          <CardDescription>Rules that determine when and where alerts are sent.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <NotificationPoliciesList policies={notificationPolicies} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
