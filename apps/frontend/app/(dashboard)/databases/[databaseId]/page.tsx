import { notFound } from 'next/navigation'
import { DatabaseOverview } from '@/components/deploycore/databases/database-overview'
import { ProductionDatabaseOverview } from '@/components/deploycore/databases/production-database-overview'
import { findDatabase } from '@/lib/databases'
import { databases as rawDatabases } from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'

const databases = getDemoFixtures(rawDatabases)

export default async function DatabaseOverviewPage({
  params,
}: {
  params: Promise<{ databaseId: string }>
}) {
  const { databaseId } = await params
  if (!isDemoModeEnabled()) {
    return <ProductionDatabaseOverview />
  }
  const database = findDatabase(databaseId, databases)
  if (!database) notFound()

  return <DatabaseOverview database={database} />
}
