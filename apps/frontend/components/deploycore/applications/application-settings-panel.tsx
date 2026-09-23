'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { apiClient, ApiError } from '@/lib/api'
import type { Application } from '@/lib/types'

interface ApplicationSettingsPanelProps {
  application: Application
}

export function ApplicationSettingsPanel({ application }: ApplicationSettingsPanelProps) {
  const router = useRouter()
  const [openDelete, setOpenDelete] = useState(false)

  return (
    <div className="flex flex-col gap-4">
      <Card size="sm">
        <CardHeader>
          <CardTitle>Application settings</CardTitle>
          <CardDescription>Identity and runtime configuration for {application.name}.</CardDescription>
        </CardHeader>
        <CardContent>
          <DetailList
            columns={2}
            items={[
              { label: 'Name', value: application.name },
              { label: 'Runtime', value: application.runtime },
              {
                label: 'Project',
                value: (
                  <Link href="/projects" className="hover:underline">
                    {application.project}
                  </Link>
                ),
              },
              { label: 'Environment', value: application.environment },
              { label: 'Server', value: application.server },
              { label: 'Instances', value: String(application.instances) },
              { label: 'Repository', value: application.repo },
              { label: 'Branch', value: application.branch },
            ]}
          />
        </CardContent>
      </Card>

      <Card size="sm" className="border-critical/30">
        <CardHeader>
          <CardTitle className="text-critical">Danger zone</CardTitle>
          <CardDescription>
            Permanently delete this application, stopping active replicas and freeing associated routes.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DestructiveConfirmDialog
            open={openDelete}
            onOpenChange={setOpenDelete}
            trigger={
              <Button variant="destructive" size="sm">
                <Trash2 data-icon="inline-start" />
                Delete application
              </Button>
            }
            title={`Delete ${application.name}?`}
            description="This action cannot be undone. Active replicas will be stopped and removed. Type the application name to confirm."
            confirmLabel="Delete application"
            confirmationPhrase={application.name.toLowerCase()}
            onConfirm={async () => {
              try {
                await apiClient.delete(`/applications/${application.id}`)
                toast.success(`Application “${application.name}” deleted`)
                router.push('/applications')
              } catch (err) {
                if (err instanceof ApiError) {
                  toast.error(err.message)
                } else {
                  toast.error(err instanceof Error ? err.message : 'Failed to delete application')
                }
              }
            }}
          />
        </CardContent>
      </Card>
    </div>
  )
}
