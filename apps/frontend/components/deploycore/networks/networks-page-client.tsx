'use client'

import { NetworksFilterTable } from '@/components/deploycore/networks/networks-filter-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import type { DockerNetwork } from '@/lib/types'

interface NetworksPageClientProps {
  networks: DockerNetwork[]
}

export function NetworksPageClient({ networks }: NetworksPageClientProps) {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Networks"
        description="Docker networks connecting services within each project and environment."
      />
      <NetworksFilterTable networks={networks} />
    </PageContainer>
  )
}
