import type { ReactNode } from 'react'
import { ServerDetailShell } from '@/components/deploycore/servers/server-detail-shell'

export default async function ServerLayout({
  children,
  params,
}: {
  children: ReactNode
  params: Promise<{ serverId: string }>
}) {
  const { serverId } = await params
  return <ServerDetailShell serverId={serverId}>{children}</ServerDetailShell>
}
