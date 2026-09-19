import { AlertCircle, RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

interface ErrorStateProps {
  title?: string
  message: string
  onRetry?: () => void
  action?: ReactNode
}

export function ErrorState({
  title = 'Something went wrong',
  message,
  onRetry,
  action,
}: ErrorStateProps) {
  return (
    <Alert variant="destructive" className="border-critical/30 bg-critical/5 text-foreground">
      <AlertCircle className="size-4 text-critical" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription className="flex flex-col gap-3">
        <span className="text-muted-foreground">{message}</span>
        {(onRetry || action) && (
          <div className="flex flex-wrap items-center gap-2">
            {onRetry && (
              <Button type="button" size="sm" variant="outline" onClick={onRetry}>
                <RefreshCw data-icon="inline-start" />
                Retry
              </Button>
            )}
            {action}
          </div>
        )}
      </AlertDescription>
    </Alert>
  )
}
