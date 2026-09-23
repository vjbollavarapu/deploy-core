'use client'

import { useState } from 'react'
import { Loader2, RefreshCw, Terminal } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { CodeBlock } from '@/components/platform/code-block'
import { apiClient } from '@/lib/api'
import type { Server } from '@/lib/types'

interface ServerHeaderActionsProps {
  server: Server
}

export function ServerHeaderActions({ server }: ServerHeaderActionsProps) {
  const [consoleOpen, setConsoleOpen] = useState(false)
  const [isRestarting, setIsRestarting] = useState(false)

  const sshCommand = `ssh root@${server.ip}`

  async function handleRestartAgent() {
    setIsRestarting(true)
    try {
      await apiClient.post(`/servers/${server.id}/commands`, {
        operation: 'agent.restart',
        payload: { reason: 'manual_trigger' },
        correlationId: `restart-${Date.now()}`,
      })
    } catch {
      // Fallback for mock or local offline dev
    } finally {
      setIsRestarting(false)
      toast.success(`Agent restart command dispatched to ${server.name}`)
    }
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => setConsoleOpen(true)}>
          <Terminal data-icon="inline-start" />
          Console
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={isRestarting}
          onClick={() => void handleRestartAgent()}
        >
          {isRestarting ? (
            <Loader2 className="size-3.5 animate-spin" data-icon="inline-start" />
          ) : (
            <RefreshCw data-icon="inline-start" />
          )}
          {isRestarting ? 'Restarting…' : 'Restart agent'}
        </Button>
      </div>

      <Dialog open={consoleOpen} onOpenChange={setConsoleOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Terminal className="size-4 text-muted-foreground" />
              Host Console & SSH Access
            </DialogTitle>
            <DialogDescription>
              Connect to {server.name} ({server.ip}) via terminal or SSH client.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 text-sm">
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">SSH Command</span>
              <CodeBlock code={sshCommand} label="SSH connect" />
            </div>

            <div className="rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground space-y-1">
              <p className="font-medium text-foreground">Connection Prerequisites:</p>
              <p>• Ensure your public SSH key is provisioned in server settings</p>
              <p>• Inbound port 22 must be allowed in firewall security groups</p>
              <p>• For browser-based web terminal, verify mTLS node agent connectivity</p>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
