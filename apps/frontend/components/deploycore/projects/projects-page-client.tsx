'use client'

import { NewProjectButton } from '@/components/deploycore/projects/project-settings-panel'
import { ProjectsTable } from '@/components/deploycore/projects/projects-table'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import type { Project } from '@/lib/types'

export function ProjectsPageClient({ projects }: { projects: Project[] }) {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Projects"
        description="Group related applications, environments, and shared infrastructure."
        actions={<NewProjectButton />}
      />
      <ProjectsTable projects={projects} />
    </PageContainer>
  )
}
