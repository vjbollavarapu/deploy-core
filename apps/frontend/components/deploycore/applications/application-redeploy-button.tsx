'use client'

import { useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Rocket, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { apiClient, ApiError } from '@/lib/api'
import { deployActionLabel, queueManualDeployment } from '@/lib/applications/application-bootstrap'

interface ApplicationRedeployButtonProps {
  applicationId: string
  applicationName: string
  status?: string
}

export function ApplicationRedeployButton({
  applicationId,
  applicationName,
  status,
}: ApplicationRedeployButtonProps) {
  const router = useRouter()
  const inflight = useRef({ current: false })
  const [isPending, setIsPending] = useState(false)
  const label = deployActionLabel(status)
  const firstDeploy = label === 'Deploy'

  async function handleDeploy() {
    if (inflight.current.current || isPending) return
    setIsPending(true)
    try {
      const result = await queueManualDeployment(apiClient, applicationId, inflight.current)
      if (result.kind === 'ignored') return
      toast.success(
        firstDeploy
          ? `Deployment queued for “${applicationName}”`
          : `Redeployment triggered for “${applicationName}”`,
      )
      if (result.deploymentId) {
        router.push(`/deployments/${result.deploymentId}`)
      }
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to queue deployment')
      }
    } finally {
      setIsPending(false)
    }
  }

  return (
    <Button size="sm" onClick={() => void handleDeploy()} disabled={isPending}>
      {firstDeploy ? (
        <Rocket data-icon="inline-start" />
      ) : (
        <RotateCcw data-icon="inline-start" className={isPending ? 'animate-spin' : undefined} />
      )}
      {isPending ? (firstDeploy ? 'Deploying…' : 'Triggering…') : label}
    </Button>
  )
}
