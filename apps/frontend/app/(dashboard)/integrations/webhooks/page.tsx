import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { WebhooksTable } from '@/components/deploycore/webhooks/webhooks-table'
import { WEBHOOK_EVENT_OPTIONS } from '@/lib/integrations'
import { webhooks } from '@/lib/mock-data'

export default function WebhooksPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Webhooks"
        description="Outbound event notifications with signing secrets, enablement, deliveries, and retries."
      />
      <Card>
        <CardHeader>
          <CardTitle>Outgoing webhooks</CardTitle>
          <CardDescription>
            Events include {WEBHOOK_EVENT_OPTIONS.slice(0, 4).join(', ')}, and more. Payloads never
            include secrets.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <WebhooksTable webhooks={webhooks} />
        </CardContent>
      </Card>
    </PageContainer>
  )
}
