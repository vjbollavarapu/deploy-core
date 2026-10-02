'use client'

import { useRef, useState } from 'react'
import { RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { apiClient, ApiError } from '@/lib/api'
import { retryProductionDatabaseProvision } from '@/lib/control-plane/database-read'

interface DatabaseProvisionRetryButtonProps {
  databaseId: string
  databaseName: string
  onRetried: () => void
}

export function DatabaseProvisionRetryButton({
  databaseId,
  databaseName,
  onRetried,
}: DatabaseProvisionRetryButtonProps) {
  const inflight = useRef({ current: false })
  const [isPending, setIsPending] = useState(false)

  async function handleRetry() {
    if (inflight.current.current || isPending) return
    setIsPending(true)
    try {
      const result = await retryProductionDatabaseProvision(apiClient, databaseId, inflight.current)
      if (result.kind === 'ignored') return
      toast.success(`Provisioning retry accepted for “${databaseName}”`)
      onRetried()
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to retry provisioning')
      }
    } finally {
      setIsPending(false)
    }
  }

  return (
    <Button size="sm" onClick={() => void handleRetry()} disabled={isPending}>
      <RotateCcw data-icon="inline-start" className={isPending ? 'animate-spin' : undefined} />
      {isPending ? 'Retrying…' : 'Retry provisioning'}
    </Button>
  )
}
