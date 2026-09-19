import { notFound } from 'next/navigation'
import { DatabaseOverview } from '@/components/deploycore/databases/database-overview'
import { findDatabase } from '@/lib/databases'
import { databases } from '@/lib/mock-data'

export default async function DatabaseOverviewPage({
  params,
}: {
  params: Promise<{ databaseId: string }>
}) {
  const { databaseId } = await params
  const database = findDatabase(databaseId, databases)
  if (!database) notFound()

  return <DatabaseOverview database={database} />
}
