'use client'

import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { toast } from 'sonner'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'

const ORG_SLUG = 'daya-inc'

export default function SettingsPage() {
  return (
    <PageContainer density="wide">
      <PageHeader title="Settings" description="Organization profile, defaults, and platform preferences." />

      <Card>
        <CardHeader>
          <CardTitle>Organization</CardTitle>
          <CardDescription>Basic information about your organization.</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="org-name">Organization name</FieldLabel>
              <Input id="org-name" defaultValue="Daya Inc." />
            </Field>
            <Field>
              <FieldLabel htmlFor="org-slug">Slug</FieldLabel>
              <Input id="org-slug" defaultValue={ORG_SLUG} className="font-mono" />
              <FieldDescription>Used in URLs and CLI references.</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="org-region">Default region</FieldLabel>
              <Select defaultValue="fra1">
                <SelectTrigger id="org-region" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="fra1">Frankfurt (fra1)</SelectItem>
                  <SelectItem value="iad1">Washington DC (iad1)</SelectItem>
                  <SelectItem value="sin1">Singapore (sin1)</SelectItem>
                  <SelectItem value="gru1">São Paulo (gru1)</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </FieldGroup>
        </CardContent>
        <CardFooter>
          <Button size="sm" onClick={() => toast.success('Organization settings saved')}>
            Save changes
          </Button>
        </CardFooter>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Platform defaults</CardTitle>
          <CardDescription>Applied to new applications unless overridden.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-4">
            <PreferenceRow
              id="auto-deploy"
              title="Auto-deploy on push"
              description="Deploy automatically when the default branch changes."
              defaultChecked
            />
            <Separator />
            <PreferenceRow
              id="zero-downtime"
              title="Zero-downtime deploys"
              description="Route traffic to the new revision only after health checks pass."
              defaultChecked
            />
            <Separator />
            <PreferenceRow
              id="preview-envs"
              title="Preview environments"
              description="Spin up an isolated environment for every pull request."
            />
          </div>
        </CardContent>
      </Card>

      <Card className="border-critical/30">
        <CardHeader>
          <CardTitle className="text-critical">Danger zone</CardTitle>
          <CardDescription>
            Deleting the organization removes every project, application, and server permanently.
          </CardDescription>
        </CardHeader>
        <CardFooter>
          <DestructiveConfirmDialog
            trigger={
              <Button variant="destructive" size="sm">
                Delete organization
              </Button>
            }
            title="Delete Daya Inc.?"
            description="This permanently deletes all projects, applications, servers, and secrets. Type the organization slug to confirm."
            confirmLabel="Delete organization"
            confirmationPhrase={ORG_SLUG}
            onConfirm={() => {
              toast.success('Organization delete queued')
            }}
          />
        </CardFooter>
      </Card>
    </PageContainer>
  )
}

function PreferenceRow({
  id,
  title,
  description,
  defaultChecked,
}: {
  id: string
  title: string
  description: string
  defaultChecked?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-0.5">
        <Label htmlFor={id} className="text-sm font-medium text-foreground">
          {title}
        </Label>
        <span className="text-xs text-muted-foreground">{description}</span>
      </div>
      <Switch id={id} defaultChecked={defaultChecked} aria-label={title} />
    </div>
  )
}
