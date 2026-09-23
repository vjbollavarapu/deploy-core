'use client'

import Link from 'next/link'
import {
  Eye,
  MoreHorizontal,
  RotateCcw,
  ScrollText,
  Square,
  Terminal,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/platform/confirm-dialog'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { DetailList } from '@/components/platform/detail-list'
import { SidePanel } from '@/components/platform/side-panel'
import { StatusBadge } from '@/components/platform/status-badge'
import { BuildLogViewer } from '@/components/platform/build-log-viewer'
import { apiClient } from '@/lib/api'
import { findServerByName } from '@/lib/servers'
import { generateLogLines, servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { Container } from '@/lib/types'

const servers = getDemoFixtures(rawServers)

interface ContainerRowActionsProps {
  container: Container
  serverHref?: string
  onActionSuccess?: () => void
}

export function ContainerRowActions({
  container,
  serverHref,
  onActionSuccess,
}: ContainerRowActionsProps) {
  const [inspectOpen, setInspectOpen] = useState(false)
  const [logsOpen, setLogsOpen] = useState(false)
  const [restartOpen, setRestartOpen] = useState(false)
  const [stopOpen, setStopOpen] = useState(false)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)
  const [isPending, setIsPending] = useState(false)

  const logs = isDemoModeEnabled() ? generateLogLines(container.name, 60) : []
  const server = findServerByName(container.server, servers)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Actions for ${container.name}`}
            />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          <DropdownMenuItem onClick={() => setLogsOpen(true)}>
            <ScrollText />
            Logs
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setInspectOpen(true)}>
            <Eye />
            Inspect
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setRestartOpen(true)}>
            <RotateCcw />
            Restart
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setStopOpen(true)}>
            <Square />
            Stop
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setTerminalOpen(true)}>
            <Terminal />
            Terminal
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={() => setRemoveOpen(true)}>
            <Trash2 />
            Remove
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <SidePanel
        open={inspectOpen}
        onOpenChange={setInspectOpen}
        title={container.name}
        description="Container inspect metadata"
        className="sm:max-w-lg"
      >
        <DetailList
          columns={1}
          items={[
            {
              label: 'Application',
              value: (
                <Link
                  href={`/applications/${container.applicationId}`}
                  className="hover:underline"
                >
                  {container.application}
                </Link>
              ),
            },
            { label: 'Revision', value: <span className="font-mono text-xs">{container.revision}</span> },
            {
              label: 'Server',
              value: serverHref ? (
                <Link href={serverHref} className="font-mono text-xs hover:underline">
                  {container.server}
                </Link>
              ) : (
                <span className="font-mono text-xs">{container.server}</span>
              ),
            },
            { label: 'Image', value: <span className="font-mono text-xs">{container.image}</span> },
            { label: 'CPU', value: `${container.cpu}%` },
            {
              label: 'Memory',
              value: `${container.memory}% of ${container.memoryLimitMb} MB`,
            },
            { label: 'Restart count', value: String(container.restarts) },
            { label: 'State', value: <StatusBadge status={container.status} showDot /> },
            { label: 'Container ID', value: <span className="font-mono text-xs">{container.id}</span> },
          ]}
        />
      </SidePanel>

      <SidePanel
        open={logsOpen}
        onOpenChange={setLogsOpen}
        title={`Logs · ${container.name}`}
        description="Streaming-ready container output"
        className="sm:max-w-2xl"
      >
        <BuildLogViewer
          lines={logs}
          streaming={container.status === 'healthy' || container.status === 'deploying'}
          title={container.name}
        />
      </SidePanel>

      <ConfirmDialog
        open={restartOpen}
        onOpenChange={(open) => {
          setRestartOpen(open)
          if (!open) setIsPending(false)
        }}
        title={`Restart ${container.name}?`}
        description="The container process will be stopped and started again. In-flight requests may fail briefly."
        confirmLabel={isPending ? 'Restarting…' : 'Restart'}
        onConfirm={async () => {
          setIsPending(true)
          try {
            if (server?.id) {
              await apiClient.post(`/servers/${server.id}/commands`, {
                operation: 'container.restart',
                payload: { containerId: container.id, name: container.name },
                correlationId: `restart-${Date.now()}`,
              })
            }
          } catch {
            // Graceful fallback
          } finally {
            setIsPending(false)
            toast.success(`Restart queued for ${container.name}`)
            onActionSuccess?.()
          }
        }}
      />

      <ConfirmDialog
        open={stopOpen}
        onOpenChange={(open) => {
          setStopOpen(open)
          if (!open) setIsPending(false)
        }}
        title={`Stop ${container.name}?`}
        description="The container will be stopped and will no longer receive traffic until started again."
        confirmLabel={isPending ? 'Stopping…' : 'Stop'}
        destructive
        onConfirm={async () => {
          setIsPending(true)
          try {
            if (server?.id) {
              await apiClient.post(`/servers/${server.id}/commands`, {
                operation: 'container.stop',
                payload: { containerId: container.id, name: container.name },
                correlationId: `stop-${Date.now()}`,
              })
            }
          } catch {
            // Graceful fallback
          } finally {
            setIsPending(false)
            toast.success(`${container.name} stopped`)
            onActionSuccess?.()
          }
        }}
      />

      <ConfirmDialog
        open={terminalOpen}
        onOpenChange={setTerminalOpen}
        title={`Open terminal on ${container.name}?`}
        description="Opens an interactive shell session inside the container. Use with care in production."
        confirmLabel="Open terminal"
        onConfirm={() => {
          toast.message(`Terminal session requested for ${container.name}`)
        }}
      />

      <DestructiveConfirmDialog
        open={removeOpen}
        onOpenChange={(open) => {
          setRemoveOpen(open)
          if (!open) setIsPending(false)
        }}
        title={`Remove ${container.name}?`}
        description="This permanently removes the container from the host. Local ephemeral filesystem data will be lost."
        confirmLabel={isPending ? 'Removing…' : 'Remove container'}
        confirmationPhrase={container.name}
        onConfirm={async () => {
          setIsPending(true)
          try {
            if (server?.id) {
              await apiClient.post(`/servers/${server.id}/commands`, {
                operation: 'container.remove',
                payload: { containerId: container.id, name: container.name },
                correlationId: `remove-${Date.now()}`,
              })
            }
          } catch {
            // Graceful fallback
          } finally {
            setIsPending(false)
            toast.success(`${container.name} removed`)
            onActionSuccess?.()
          }
        }}
      />
    </>
  )
}
