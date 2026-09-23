'use client'

import { useCallback, useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { NotificationChannelsList } from '@/components/deploycore/notifications/notification-channels-list'
import { NotificationPoliciesList } from '@/components/deploycore/notifications/notification-policies-list'
import { AddChannelDialog } from '@/components/deploycore/notifications/add-channel-dialog'
import { AddPolicyDialog } from '@/components/deploycore/notifications/add-policy-dialog'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import {
  fetchNotificationChannels,
  fetchNotificationPolicies,
  mapWireNotificationChannel,
  mapWireNotificationPolicy,
} from '@/lib/integrations'
import {
  notificationChannels as rawNotificationChannels,
  notificationPolicies as rawNotificationPolicies,
} from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { NotificationChannel, NotificationPolicy } from '@/lib/types'

export default function NotificationsPage() {
  const { activeOrg } = useOrganization()
  const [addChannelOpen, setAddChannelOpen] = useState(false)
  const [addPolicyOpen, setAddPolicyOpen] = useState(false)

  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  const loadChannels = useCallback(async (): Promise<NotificationChannel[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawNotificationChannels) : []
    try {
      const res = await fetchNotificationChannels(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map((c) => mapWireNotificationChannel(c))
      }
      return isDemo ? getDemoFixtures(rawNotificationChannels) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawNotificationChannels)
      throw err
    }
  }, [orgId, isDemo])

  const {
    data: channelsData,
    isLoading: loadingChannels,
    error: channelsError,
    reload: reloadChannels,
  } = useApiQuery(loadChannels)

  const channels = useMemo(() => channelsData || [], [channelsData])

  const channelMap = useMemo(() => {
    const map: Record<string, string> = {}
    channels.forEach((c) => {
      map[c.id] = c.name
    })
    return map
  }, [channels])

  const loadPolicies = useCallback(async (): Promise<NotificationPolicy[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawNotificationPolicies) : []
    try {
      const res = await fetchNotificationPolicies(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map((p) => mapWireNotificationPolicy(p, channelMap))
      }
      return isDemo ? getDemoFixtures(rawNotificationPolicies) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawNotificationPolicies)
      throw err
    }
  }, [orgId, isDemo, channelMap])

  const {
    data: policiesData,
    isLoading: loadingPolicies,
    error: policiesError,
    reload: reloadPolicies,
  } = useApiQuery(loadPolicies)

  const policies = policiesData || []

  const refetchAll = () => {
    reloadChannels()
    reloadPolicies()
  }

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Notifications"
        description="Alert channels and policies that route deployment, server, backup, and certificate events."
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => setAddPolicyOpen(true)}
            >
              <Plus className="size-4" />
              Add Policy
            </Button>
            <Button
              size="sm"
              className="gap-1.5"
              onClick={() => setAddChannelOpen(true)}
            >
              <Plus className="size-4" />
              Add Channel
            </Button>
          </div>
        }
      />

      <div className="flex flex-col gap-6">
        <Card>
          <CardHeader>
            <CardTitle>Channels</CardTitle>
            <CardDescription>
              Email, Slack, Microsoft Teams, Discord, Telegram, Webhook, and WhatsApp destinations.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {loadingChannels ? (
              <div className="py-12">
                <LoadingState label="Loading notification channels…" />
              </div>
            ) : channelsError && !isDemo ? (
              <div className="p-6">
                <ErrorState
                  title="Failed to load channels"
                  message={channelsError}
                  onRetry={reloadChannels}
                />
              </div>
            ) : (
              <NotificationChannelsList
                channels={channels}
                organizationId={orgId}
                onChannelChange={reloadChannels}
                onAddChannel={() => setAddChannelOpen(true)}
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Policies</CardTitle>
            <CardDescription>Rules that determine when and where alerts are sent.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {loadingPolicies ? (
              <div className="py-12">
                <LoadingState label="Loading notification policies…" />
              </div>
            ) : policiesError && !isDemo ? (
              <div className="p-6">
                <ErrorState
                  title="Failed to load policies"
                  message={policiesError}
                  onRetry={reloadPolicies}
                />
              </div>
            ) : (
              <NotificationPoliciesList
                policies={policies}
                onPolicyChange={reloadPolicies}
                onAddPolicy={() => setAddPolicyOpen(true)}
              />
            )}
          </CardContent>
        </Card>
      </div>

      <AddChannelDialog
        open={addChannelOpen}
        onOpenChange={setAddChannelOpen}
        organizationId={orgId}
        onSuccess={refetchAll}
      />

      <AddPolicyDialog
        open={addPolicyOpen}
        onOpenChange={setAddPolicyOpen}
        organizationId={orgId}
        availableChannels={channels}
        onSuccess={refetchAll}
      />
    </PageContainer>
  )
}
