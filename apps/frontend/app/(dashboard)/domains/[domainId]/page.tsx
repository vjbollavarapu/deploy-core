import { notFound } from 'next/navigation'
import { DomainDetailClient } from '@/components/deploycore/domains/domain-detail-client'
import { findDomain } from '@/lib/domains'
import { domains as rawDomains } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const domains = getDemoFixtures(rawDomains)

export default async function DomainDetailPage({
  params,
}: {
  params: Promise<{ domainId: string }>
}) {
  const { domainId } = await params
  const domain = findDomain(domainId, domains)
  if (!domain) notFound()

  return <DomainDetailClient domain={domain} />
}

