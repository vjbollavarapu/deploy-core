import { notFound } from 'next/navigation'
import { ServerOverview } from '@/components/deploycore/servers/server-overview'
import { findServer } from '@/lib/servers'
import { servers as rawServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const servers = getDemoFixtures(rawServers)

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
