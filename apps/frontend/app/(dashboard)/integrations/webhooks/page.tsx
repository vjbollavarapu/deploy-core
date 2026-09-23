'use client'

import { useCallback, useState } from 'react'
import { Plus, Webhook as WebhookIcon } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { EmptyState } from '@/components/platform/empty-state'
import { WebhooksTable } from '@/components/deploycore/webhooks/webhooks-table'
import { CreateWebhookDialog } from '@/components/deploycore/webhooks/create-webhook-dialog'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import {
  fetchWebhooks,
  mapWireWebhook,
  WEBHOOK_EVENT_OPTIONS,
} from '@/lib/integrations'
import { webhooks as rawWebhooks } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { Webhook } from '@/lib/types'

export default function WebhooksPage() {
  const { activeOrg } = useOrganization()
  const [createOpen, setCreateOpen] = useState(false)

  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  const loadWebhooks = useCallback(async (): Promise<Webhook[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawWebhooks) : []
    try {
      const res = await fetchWebhooks(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map((w) => mapWireWebhook(w))
      }
      return isDemo ? getDemoFixtures(rawWebhooks) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawWebhooks)
      throw err
    }
  }, [orgId, isDemo])

  const { data, isLoading, error, reload } = useApiQuery(loadWebhooks)
  const webhooks = data || []

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Webhooks"
        description="Outbound event notifications with signing secrets, enablement, deliveries, and retries."
        actions={
          <Button
            size="sm"
            className="gap-1.5"
            onClick={() => setCreateOpen(true)}
          >
            <Plus className="size-4" />
            Create Webhook
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Outgoing Webhooks</CardTitle>
          <CardDescription>
            Events include {WEBHOOK_EVENT_OPTIONS.slice(0, 4).join(', ')}, and more. Payloads are signed with HMAC-SHA256 and never include secrets.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="py-12">
              <LoadingState label="Loading outgoing webhooks…" />
            </div>
          ) : error && !isDemo ? (
            <div className="p-6">
              <ErrorState
                title="Failed to load webhooks"
                message={error}
                onRetry={reload}
              />
            </div>
          ) : webhooks.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={WebhookIcon}
                title="No webhooks configured"
                description="Register HTTP endpoints to receive signed notifications whenever deployments run, nodes go offline, or backups complete."
                action={
                  <Button size="sm" onClick={() => setCreateOpen(true)} className="gap-1.5">
                    <Plus className="size-4" />
                    Create Webhook
                  </Button>
                }
              />
            </div>
          ) : (
            <WebhooksTable
              webhooks={webhooks}
              organizationId={orgId}
              onWebhookChange={reload}
            />
          )}
        </CardContent>
      </Card>

      <CreateWebhookDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        organizationId={orgId}
        onSuccess={reload}
      />
    </PageContainer>
  )
}
