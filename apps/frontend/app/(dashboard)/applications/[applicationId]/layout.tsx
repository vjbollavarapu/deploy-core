import type { ReactNode } from 'react'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { GitBranch } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ApplicationRedeployButton } from '@/components/deploycore/applications/application-redeploy-button'
import { ApplicationSubnav } from '@/components/platform/application-subnav'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { StatusBadge } from '@/components/platform/status-badge'
import { findApplication, getPrimaryDomain } from '@/lib/applications'
import { projects as rawProjects } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const projects = getDemoFixtures(rawProjects)

export default async function ApplicationLayout({
  children,
  params,
}: {
  children: ReactNode
  params: Promise<{ applicationId: string }>
}) {
  const { applicationId } = await params
  const application = findApplication(applicationId)
  if (!application) notFound()

  const project = projects.find((p) => p.id === application.projectId)
  const primaryDomain = getPrimaryDomain(application)

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={application.name}
        description={`${application.runtime} on ${application.server}`}
        breadcrumbs={[
          { label: 'Applications', href: '/applications' },
          ...(project
            ? [{ label: project.name, href: `/projects/${project.slug}` }]
            : [{ label: application.project }]),
          { label: application.name },
        ]}
        badges={
          <>
            <StatusBadge status={application.status} />
            <EnvironmentBadge environment={application.environment} />
          </>
        }
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="font-mono">{application.revision}</span>
            {primaryDomain ? (
              <a
                href={`https://${primaryDomain.domain}`}
                target="_blank"
                rel="noreferrer"
                className="font-mono hover:text-foreground hover:underline"
              >
                {primaryDomain.domain}
              </a>
            ) : null}
            <span>{application.lastDeployment}</span>
          </div>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<Link href={`/applications/${application.id}/deployments`} />}
            >
              <GitBranch data-icon="inline-start" />
              Deployments
            </Button>
            <ApplicationRedeployButton
              applicationId={application.id}
              applicationName={application.name}
            />
          </div>
        }
      />
      <ApplicationSubnav applicationId={applicationId} />
      {children}
    </PageContainer>
  )
}
