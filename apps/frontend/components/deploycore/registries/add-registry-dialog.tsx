'use client'

import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { createRegistry } from '@/lib/integrations'
import {
  createRegistrySchema,
  type CreateRegistryValues,
} from '@/lib/validations/integration'

interface AddRegistryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  organizationId: string
  onSuccess: () => void
}

const URL_PRESETS: Record<string, string> = {
  ghcr: 'ghcr.io',
  dockerhub: 'docker.io',
  ecr: '123456789012.dkr.ecr.us-east-1.amazonaws.com',
  gcp: 'us-central1-docker.pkg.dev/my-project/my-repo',
  acr: 'myregistry.azurecr.io',
  oci: 'registry.example.com',
}

export function AddRegistryDialog({
  open,
  onOpenChange,
  organizationId,
  onSuccess,
}: AddRegistryDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<CreateRegistryValues>({
    resolver: zodResolver(createRegistrySchema),
    defaultValues: {
      name: '',
      provider: 'ghcr',
      registryUrl: 'ghcr.io',
      username: '',
      token: '',
    },
  })

  const selectedProvider = watch('provider')

  useEffect(() => {
    if (selectedProvider && URL_PRESETS[selectedProvider]) {
      setValue('registryUrl', URL_PRESETS[selectedProvider], { shouldValidate: true })
    }
  }, [selectedProvider, setValue])

  const onSubmit = async (values: CreateRegistryValues) => {
    try {
      setSubmitting(true)
      await createRegistry({
        organizationId,
        name: values.name,
        provider: values.provider,
        registryUrl: values.registryUrl,
        username: values.username,
        credentials: {
          username: values.username,
          token: values.token,
        },
      })
      toast.success(`Registry "${values.name}" configured successfully`)
      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      toast.error('Failed to configure container registry', {
        description: err instanceof Error ? err.message : 'Please check credentials and retry.',
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add Container Registry</DialogTitle>
          <DialogDescription>
            Register credentials for pulling and pushing container images (GHCR, Docker Hub, ECR, GCP, ACR, or generic OCI).
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="registry-name">Registry Name</Label>
            <Input
              id="registry-name"
              placeholder="e.g. Production GHCR or Acme Docker Hub"
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-destructive">{errors.name.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="registry-provider">Provider Type</Label>
            <Select
              value={selectedProvider}
              onValueChange={(val) => setValue('provider', val as CreateRegistryValues['provider'], { shouldValidate: true })}
            >
              <SelectTrigger id="registry-provider">
                <SelectValue placeholder="Select provider" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ghcr">GitHub Container Registry (GHCR)</SelectItem>
                <SelectItem value="dockerhub">Docker Hub</SelectItem>
                <SelectItem value="ecr">AWS Elastic Container Registry (ECR)</SelectItem>
                <SelectItem value="gcp">Google Cloud Artifact Registry (GCP)</SelectItem>
                <SelectItem value="acr">Azure Container Registry (ACR)</SelectItem>
                <SelectItem value="oci">Generic OCI Compliant Registry</SelectItem>
              </SelectContent>
            </Select>
            {errors.provider && (
              <p className="text-xs text-destructive">{errors.provider.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="registry-url">Registry Server URL</Label>
            <Input
              id="registry-url"
              placeholder={URL_PRESETS[selectedProvider] || 'registry.example.com'}
              {...register('registryUrl')}
            />
            {errors.registryUrl && (
              <p className="text-xs text-destructive">{errors.registryUrl.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="registry-username">Username / Access Key</Label>
            <Input
              id="registry-username"
              placeholder={selectedProvider === 'ecr' ? 'AWS' : 'username or robot-account'}
              {...register('username')}
            />
            {errors.username && (
              <p className="text-xs text-destructive">{errors.username.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="registry-token">Password / Access Token</Label>
            <Input
              id="registry-token"
              type="password"
              placeholder="••••••••••••••••••••••••"
              {...register('token')}
            />
            <p className="text-[11px] text-muted-foreground">
              Write-only credential sealed with AES-256-GCM. Never returned over HTTP.
            </p>
            {errors.token && (
              <p className="text-xs text-destructive">{errors.token.message}</p>
            )}
          </div>

          <DialogFooter className="pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Saving…' : 'Add Registry'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
