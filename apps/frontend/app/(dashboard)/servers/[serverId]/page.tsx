import { notFound } from 'next/navigation'
import { ServerOverview } from '@/components/deploycore/servers/server-overview'
import { findServer } from '@/lib/servers'
import { servers } from '@/lib/mock-data'

export default async function ServerOverviewPage({
  params,
}: {
  params: Promise<{ serverId: string }>
}) {
  const { serverId } = await params
  const server = findServer(serverId, servers)
  if (!server) notFound()

  return <ServerOverview server={server} />
}
