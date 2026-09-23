'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { RotateCcw, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import type { Deployment } from '@/lib/types'

interface DeploymentActionsProps {
  deployment: Deployment
}

export function DeploymentActions({ deployment }: DeploymentActionsProps) {
  const router = useRouter()
  const [isCancelling, setIsCancelling] = useState(false)
  const [isRedeploying, setIsRedeploying] = useState(false)
  const [openCancelDialog, setOpenCancelDialog] = useState(false)

  const isCancelable =
    deployment.status === 'deploying' ||
    deployment.status === 'queued' ||
    deployment.status === 'pending'

  async function handleCancel() {
    setIsCancelling(true)
    try {
      await apiClient.post(`/deployments/${deployment.id}/cancel`)
      toast.success(`Deployment #${deployment.number} cancelled`)
      router.refresh()
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to cancel deployment')
      }
    } finally {
      setIsCancelling(false)
    }
  }

  async function handleRedeploy() {
    setIsRedeploying(true)
    try {
      const res = await apiClient.post<{ deployment?: { id: string } }>(
        `/applications/${deployment.applicationId}/deployments`,
        { trigger: 'manual' },
      )
      toast.success(`Redeployment triggered for ${deployment.application}`)
      if (res?.deployment?.id) {
        router.push(`/deployments/${res.deployment.id}`)
      } else {
        router.refresh()
      }
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to trigger redeployment')
      }
    } finally {
      setIsRedeploying(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {isCancelable && (
        <DestructiveConfirmDialog
          open={openCancelDialog}
          onOpenChange={setOpenCancelDialog}
          trigger={
            <Button size="sm" variant="outline" disabled={isCancelling}>
              <XCircle data-icon="inline-start" />
              {isCancelling ? 'Cancelling…' : 'Cancel'}
            </Button>
          }
          title={`Cancel deployment #${deployment.number}?`}
          description="Cancelling this deployment will stop the active build or deployment container on the server."
          confirmLabel="Cancel deployment"
          onConfirm={handleCancel}
        />
      )}

      <Button size="sm" onClick={handleRedeploy} disabled={isRedeploying}>
        <RotateCcw data-icon="inline-start" className={isRedeploying ? 'animate-spin' : undefined} />
        {isRedeploying ? 'Queueing…' : 'Redeploy'}
      </Button>
    </div>
  )
}
