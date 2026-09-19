'use client'

import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent } from '@/components/ui/card'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { ApplicationsTable } from '@/components/deploycore/applications/applications-table'
import {
  EnvironmentApplicationsHealth,
  EnvironmentDatabasesPanel,
  EnvironmentDomainsPanel,
  EnvironmentHealthSummary,
  EnvironmentSecretsPanel,
  EnvironmentVariablesPanel,
} from '@/components/deploycore/projects/environment-detail-panels'
import { environmentSlug } from '@/lib/projects'
import type {
  Application,
  DatabaseInstance,
  DomainRecord,
  EnvVarEntry,
  Project,
  SecretItem,
  Status,
} from '@/lib/types'

interface EnvironmentDetailClientProps {
  project: Project
  environment: string
  health: Status
  applications: Application[]
  databases: DatabaseInstance[]
  variables: EnvVarEntry[]
  secrets: SecretItem[]
  domains: DomainRecord[]
}

export function EnvironmentDetailClient({
  project,
  environment,
  health,
  applications,
  databases,
  variables,
  secrets,
  domains,
}: EnvironmentDetailClientProps) {
  const [pendingDelete, setPendingDelete] = useState(false)

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={environment}
        description={`${project.name} · environment inventory and health`}
        breadcrumbs={[
          { label: 'Projects', href: '/projects' },
          { label: project.name, href: `/projects/${project.slug}` },
          { label: environment },
        ]}
        badges={
          <>
            <EnvironmentBadge environment={environment} />
            <StatusBadge status={health} />
          </>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm">
              <Plus data-icon="inline-start" />
              Application
            </Button>
            <DestructiveConfirmDialog
              open={pendingDelete}
              onOpenChange={setPendingDelete}
              trigger={
                <Button size="sm" variant="outline" className="text-critical">
                  <Trash2 data-icon="inline-start" />
                  Delete
                </Button>
              }
              title={`Delete ${environment}?`}
              description={`This removes the ${environment} environment from ${project.name}. Confirm by typing the environment slug.`}
              confirmLabel="Delete environment"
              confirmationPhrase={environmentSlug(environment)}
              onConfirm={() => {
                toast.success(`${environment} marked for deletion`)
              }}
            />
          </div>
        }
      />

      <EnvironmentHealthSummary
        health={health}
        applicationCount={applications.length}
        databaseCount={databases.length}
        domainCount={domains.length}
        variableCount={variables.length}
        secretCount={secrets.length}
      />

      <Tabs defaultValue="applications">
        <TabsList variant="line" className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="applications">Applications</TabsTrigger>
          <TabsTrigger value="databases">Databases</TabsTrigger>
          <TabsTrigger value="variables">Variables</TabsTrigger>
          <TabsTrigger value="secrets">Secrets</TabsTrigger>
          <TabsTrigger value="domains">Domains</TabsTrigger>
          <TabsTrigger value="health">Health</TabsTrigger>
        </TabsList>

        <TabsContent value="applications" className="mt-4">
          <Card size="sm">
            <CardContent className="p-0">
              <ApplicationsTable applications={applications} showProject={false} />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="databases" className="mt-4">
          <EnvironmentDatabasesPanel databases={databases} />
        </TabsContent>

        <TabsContent value="variables" className="mt-4">
          <EnvironmentVariablesPanel variables={variables} />
        </TabsContent>

        <TabsContent value="secrets" className="mt-4">
          <EnvironmentSecretsPanel secrets={secrets} />
        </TabsContent>

        <TabsContent value="domains" className="mt-4">
          <EnvironmentDomainsPanel domains={domains} />
        </TabsContent>

        <TabsContent value="health" className="mt-4">
          <EnvironmentApplicationsHealth applications={applications} />
        </TabsContent>
      </Tabs>
    </PageContainer>
  )
}
