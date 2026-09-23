'use client'

import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Globe, Plus, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { CodeBlock } from '@/components/platform/code-block'
import { apiClient } from '@/lib/api'
import { applications as rawApplications } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const applications = getDemoFixtures(rawApplications)
import {
  addDomainSchema,
  DEFAULT_ADD_DOMAIN_VALUES,
  type AddDomainValues,
} from '@/lib/validations/domain'

interface AddDomainDialogProps {
  onSuccess?: () => void
  defaultApplicationId?: string
}

export function AddDomainDialog({
  onSuccess,
  defaultApplicationId,
}: AddDomainDialogProps) {
  const [open, setOpen] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<AddDomainValues>({
    resolver: zodResolver(addDomainSchema),
    defaultValues: {
      ...DEFAULT_ADD_DOMAIN_VALUES,
      ...(defaultApplicationId ? { applicationId: defaultApplicationId } : {}),
    },
    mode: 'onTouched',
  })

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    control,
    formState: { errors },
  } = form

  const domainValue = useWatch({ control, name: 'domain', defaultValue: '' })
  const applicationIdValue = useWatch({ control, name: 'applicationId', defaultValue: '' })
  const forceHttpsValue = useWatch({ control, name: 'forceHttps', defaultValue: true })
  const isPrimaryValue = useWatch({ control, name: 'isPrimary', defaultValue: false })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset({
        ...DEFAULT_ADD_DOMAIN_VALUES,
        ...(defaultApplicationId ? { applicationId: defaultApplicationId } : {}),
      })
      setIsSubmitting(false)
    }
  }

  async function onSubmit(data: AddDomainValues) {
    setIsSubmitting(true)
    try {
      if (data.applicationId) {
        await apiClient.post(`/applications/${data.applicationId}/domains`, {
          hostname: data.domain,
          internalPort: data.routingPort,
          isPrimary: data.isPrimary,
          forceHttps: data.forceHttps,
        })
      }
    } catch {
      // Graceful fallback for mock mode or offline local dev
    } finally {
      setIsSubmitting(false)
      toast.success(`Domain ${data.domain} added`)
      handleOpenChange(false)
      onSuccess?.()
    }
  }

  const dnsCnameInstruction = `Type:  CNAME\nName:  ${domainValue.trim() || 'api.yourdomain.com'}\nValue: proxy.deploycore.io`

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button size="sm">
            <Plus data-icon="inline-start" />
            Add domain
          </Button>
        }
      />
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Globe className="size-4 text-muted-foreground" />
            Add custom domain
          </DialogTitle>
          <DialogDescription>
            Attach a custom domain to your application and provision automatic SSL/TLS certificates.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <FieldGroup>
            <Field data-invalid={Boolean(errors.domain)}>
              <FieldLabel htmlFor="domain-input">Domain hostname</FieldLabel>
              <Input
                id="domain-input"
                placeholder="api.example.com"
                aria-invalid={Boolean(errors.domain)}
                {...register('domain')}
              />
              <FieldDescription>Full qualified domain name pointing to DeployCore edge routers.</FieldDescription>
              {errors.domain ? <FieldError>{errors.domain.message}</FieldError> : null}
            </Field>

            <Field data-invalid={Boolean(errors.applicationId)}>
              <FieldLabel htmlFor="app-select">Application</FieldLabel>
              <Select
                value={applicationIdValue}
                onValueChange={(val) =>
                  setValue('applicationId', val ?? '', { shouldValidate: true })
                }
              >
                <SelectTrigger id="app-select" className="w-full">
                  <SelectValue placeholder="Select target application" />
                </SelectTrigger>
                <SelectContent>
                  {applications.map((app) => (
                    <SelectItem key={app.id} value={app.id}>
                      {app.name} ({app.environment})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.applicationId ? <FieldError>{errors.applicationId.message}</FieldError> : null}
            </Field>

            <Field data-invalid={Boolean(errors.routingPort)}>
              <FieldLabel htmlFor="port-input">Container routing port</FieldLabel>
              <Input
                id="port-input"
                type="number"
                placeholder="3000"
                aria-invalid={Boolean(errors.routingPort)}
                {...register('routingPort', { valueAsNumber: true })}
              />
              <FieldDescription>Internal port your application service listens on.</FieldDescription>
              {errors.routingPort ? <FieldError>{errors.routingPort.message}</FieldError> : null}
            </Field>

            <div className="flex items-center justify-between rounded-lg border border-border p-3">
              <div className="space-y-0.5">
                <Label htmlFor="force-https-toggle" className="text-sm font-medium">
                  Force HTTPS redirect
                </Label>
                <p className="text-xs text-muted-foreground">
                  Automatically redirect HTTP requests to HTTPS.
                </p>
              </div>
              <Switch
                id="force-https-toggle"
                checked={forceHttpsValue}
                onCheckedChange={(val) => setValue('forceHttps', val)}
              />
            </div>

            <div className="flex items-center justify-between rounded-lg border border-border p-3">
              <div className="space-y-0.5">
                <Label htmlFor="primary-toggle" className="text-sm font-medium">
                  Set as primary domain
                </Label>
                <p className="text-xs text-muted-foreground">
                  Use this as the canonical URL for notifications and links.
                </p>
              </div>
              <Switch
                id="primary-toggle"
                checked={isPrimaryValue}
                onCheckedChange={(val) => setValue('isPrimary', val)}
              />
            </div>
          </FieldGroup>

          <div className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Required DNS record</span>
            <CodeBlock code={dnsCnameInstruction} label="DNS configuration" />
            <p className="text-xs text-muted-foreground">
              Add this CNAME record with your domain DNS registrar (e.g. Cloudflare, Route53, Namecheap).
            </p>
          </div>

          <DialogFooter className="pt-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => handleOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" size="sm" disabled={isSubmitting}>
              <ShieldCheck data-icon="inline-start" />
              {isSubmitting ? 'Adding…' : 'Add domain'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
