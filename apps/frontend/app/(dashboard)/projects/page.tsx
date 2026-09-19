import { projects } from '@/lib/mock-data'
import { ProjectsPageClient } from '@/components/deploycore/projects/projects-page-client'

export default function ProjectsPage() {
  return <ProjectsPageClient projects={projects} />
}
