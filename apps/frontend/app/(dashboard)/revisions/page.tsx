import { RevisionsPageClient } from '@/components/deploycore/revisions/revisions-page-client'
import { revisions } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function RevisionsPage() {
  return <RevisionsPageClient revisions={getDemoFixtures(revisions)} />
}
