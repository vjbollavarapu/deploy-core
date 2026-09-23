'use client'

import { useState } from 'react'
import { History, Plus, Rocket } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { Breadcrumbs } from '@/components/platform/breadcrumbs'
import { EmptyState } from '@/components/platform/empty-state'
import { StatusBadge } from '@/components/platform/status-badge'
import { ApplicationsTable } from '@/components/deploycore/applications/applications-table'
import { CreateApplicationWizard } from '@/components/deploycore/applications/create-application-wizard'
import { DeploymentsTable } from '@/components/deploycore/deployments/deployments-table'
import { ProjectOverview } from '@/components/deploycore/projects/project-overview'
import { ProjectEnvironmentsPanel } from '@/components/deploycore/projects/project-environments-panel'
import { ProjectResourcesPanel } from '@/components/deploycore/projects/project-resources-panel'
import { ProjectSettingsPanel } from '@/components/deploycore/projects/project-settings-panel'
import { EnvironmentFormDialog } from '@/components/deploycore/projects/environment-form-dialog'
import type {
  Application,
  DatabaseInstance,
  Deployment,
  DomainRecord,
  DockerNetwork,
  Project,
  SecretItem,
  Status,
  Volume,
} from '@/lib/types'

interface ProjectDetailClientProps {
  project: Project
  applications: Application[]
  deployments: Deployment[]
  resources: {
    databases: DatabaseInstance[]
    volumes: Volume[]
    networks: DockerNetwork[]
    domains: DomainRecord[]
    secrets: SecretItem[]
  }
  environmentHealth: Record<string, Status>
}

export function ProjectDetailClient({
  project,
  applications,
  deployments,
  resources,
  environmentHealth,
}: ProjectDetailClientProps) {
  const [envDialogOpen, setEnvDialogOpen] = useState(false)

  return (
    <PageContainer density="wide">
      <PageHeader
        title={project.name}
        description={`Owned by ${project.owner.name}`}
        breadcrumbs={
          <Breadcrumbs
            items={[
              { label: 'Projects', href: '/projects' },
              { label: project.name },
            ]}
          />
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={project.health} />
            <Button size="sm" variant="outline" onClick={() => setEnvDialogOpen(true)}>
              <Plus data-icon="inline-start" />
              Environment
            </Button>
            <CreateApplicationWizard
              defaultProjectId={project.id}
              trigger={
                <Button size="sm">
                  <Plus data-icon="inline-start" />
                  Application
                </Button>
              }
            />
          </div>
        }
      />

      <Tabs defaultValue="overview">
        <TabsList variant="line" className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="environments">Environments</TabsTrigger>
          <TabsTrigger value="applications">Applications</TabsTrigger>
          <TabsTrigger value="resources">Resources</TabsTrigger>
          <TabsTrigger value="activity">Recent activity</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <ProjectOverview
            project={project}
            applicationCount={applications.length}
            databaseCount={resources.databases.length}
            domainCount={resources.domains.length}
            secretCount={resources.secrets.length}
            environmentHealth={environmentHealth}
            recentApplications={applications}
          />
        </TabsContent>

        <TabsContent value="environments" className="mt-4">
          <ProjectEnvironmentsPanel
            project={project}
            applications={applications}
            environmentHealth={environmentHealth}
          />
        </TabsContent>

        <TabsContent value="applications" className="mt-4">
          <Card size="sm">
            <CardContent className="p-0">
              {applications.length === 0 ? (
                <EmptyState
                  icon={Rocket}
                  title="No applications"
                  description="This project doesn't have any applications yet."
                  action={
                    <CreateApplicationWizard
                      defaultProjectId={project.id}
                      trigger={
                        <Button size="sm">
                          <Plus data-icon="inline-start" />
                          Application
                        </Button>
                      }
                    />
                  }
                  className="border-0 p-8"
                />
              ) : (
                <ApplicationsTable applications={applications} showProject={false} />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="resources" className="mt-4">
          <ProjectResourcesPanel {...resources} />
        </TabsContent>

        <TabsContent value="activity" className="mt-4">
          <Card size="sm">
            <CardContent className="p-0">
              {deployments.length === 0 ? (
                <EmptyState
                  icon={History}
                  title="No deployment activity"
                  description="Deployments created in this project will appear here."
                  className="border-0 p-8"
                />
              ) : (
                <DeploymentsTable deployments={deployments} />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings" className="mt-4">
          <ProjectSettingsPanel
            id={project.id}
            name={project.name}
            slug={project.slug}
            description={project.description}
          />
        </TabsContent>
      </Tabs>

      <EnvironmentFormDialog
        open={envDialogOpen}
        onOpenChange={setEnvDialogOpen}
        projectName={project.name}
        projectId={project.id}
        existingNames={project.environments}
      />
    </PageContainer>
  )
}
