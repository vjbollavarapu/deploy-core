'use client'

import { useState } from 'react'
import { RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { apiClient, ApiError } from '@/lib/api'

interface ApplicationRedeployButtonProps {
  applicationId: string
  applicationName: string
}

export function ApplicationRedeployButton({
  applicationId,
  applicationName,
}: ApplicationRedeployButtonProps) {
  const [isPending, setIsPending] = useState(false)

  async function handleRedeploy() {
    setIsPending(true)
    try {
      await apiClient.post(`/applications/${applicationId}/deployments`, {
        trigger: 'manual',
      })
      toast.success(`Redeployment triggered for “${applicationName}”`)
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to trigger redeployment')
      }
    } finally {
      setIsPending(false)
    }
  }

  return (
    <Button size="sm" onClick={handleRedeploy} disabled={isPending}>
      <RotateCcw data-icon="inline-start" className={isPending ? 'animate-spin' : undefined} />
      {isPending ? 'Triggering…' : 'Redeploy'}
    </Button>
  )
}
