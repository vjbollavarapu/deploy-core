'use client'

import { useCallback, useState } from 'react'
import { ContainersFilterTable } from '@/components/deploycore/containers/containers-filter-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import type { Container } from '@/lib/types'

interface ContainersPageClientProps {
  containers: Container[]
}

export function ContainersPageClient({ containers: initialContainers }: ContainersPageClientProps) {
  const [containerList, setContainerList] = useState<Container[]>(initialContainers)

  const reload = useCallback(() => {
    // Refresh container state or re-read fallback
    setContainerList((prev) => [...prev])
  }, [])

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Containers"
        description="Every container across the fleet — inspect, restart, stop, or remove with confirmation."
      />
      <ContainersFilterTable containers={containerList} onActionSuccess={reload} />
    </PageContainer>
  )
}
