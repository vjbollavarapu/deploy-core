import { projects } from '@/lib/mock-data'
import { ProjectsPageClient } from '@/components/deploycore/projects/projects-page-client'
import { getDemoFixtures } from '@/lib/mock-isolation'

export default function ProjectsPage() {
  return <ProjectsPageClient projects={getDemoFixtures(projects)} />
}
