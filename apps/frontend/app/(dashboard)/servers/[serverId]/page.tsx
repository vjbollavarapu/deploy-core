'use client'

import { ServerOverview } from '@/components/deploycore/servers/server-overview'
import { useServerDetail } from '@/components/deploycore/servers/server-detail-shell'

export default function ServerOverviewPage() {
  const { server } = useServerDetail()
  return <ServerOverview server={server} />
}
