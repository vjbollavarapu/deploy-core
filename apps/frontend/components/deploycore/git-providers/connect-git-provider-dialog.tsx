'use client'

import { useState } from 'react'
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
import { createGitConnection } from '@/lib/integrations'
import {
  createGitConnectionSchema,
  type CreateGitConnectionValues,
} from '@/lib/validations/integration'

interface ConnectGitProviderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  organizationId: string
  onSuccess: () => void
}

export function ConnectGitProviderDialog({
  open,
  onOpenChange,
  organizationId,
  onSuccess,
}: ConnectGitProviderDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<CreateGitConnectionValues>({
    resolver: zodResolver(createGitConnectionSchema),
    defaultValues: {
      provider: 'github',
      accountLogin: '',
      displayName: '',
      accessToken: '',
      webhookSecret: '',
    },
  })

  const selectedProvider = watch('provider')

  const onSubmit = async (values: CreateGitConnectionValues) => {
    try {
      setSubmitting(true)
      await createGitConnection({
        organizationId,
        provider: values.provider,
        accountLogin: values.accountLogin,
        displayName: values.displayName || values.accountLogin,
        accessToken: values.accessToken,
        webhookSecret: values.webhookSecret || undefined,
      })
      toast.success(`${values.displayName || values.accountLogin} connected successfully`)
      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      toast.error('Failed to connect Git provider', {
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
          <DialogTitle>Connect Git Provider</DialogTitle>
          <DialogDescription>
            Authorize DeployCore to synchronize repositories, read commit revisions, and configure build triggers.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="git-provider-type">Provider Type</Label>
            <Select
              value={selectedProvider}
              onValueChange={(val) => setValue('provider', val as CreateGitConnectionValues['provider'], { shouldValidate: true })}
            >
              <SelectTrigger id="git-provider-type">
                <SelectValue placeholder="Select Git host" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="github">GitHub</SelectItem>
                <SelectItem value="gitlab">GitLab</SelectItem>
                <SelectItem value="bitbucket">Bitbucket</SelectItem>
                <SelectItem value="generic">Generic Git</SelectItem>
              </SelectContent>
            </Select>
            {errors.provider && (
              <p className="text-xs text-destructive">{errors.provider.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="git-account-login">
              {selectedProvider === 'generic' ? 'Host / Account identifier' : 'Account or Organization Login'}
            </Label>
            <Input
              id="git-account-login"
              placeholder={selectedProvider === 'github' ? 'e.g. acme-corp or octocat' : 'e.g. team-name'}
              {...register('accountLogin')}
            />
            {errors.accountLogin && (
              <p className="text-xs text-destructive">{errors.accountLogin.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="git-display-name">Display Name</Label>
            <Input
              id="git-display-name"
              placeholder="e.g. Production GitHub"
              {...register('displayName')}
            />
            {errors.displayName && (
              <p className="text-xs text-destructive">{errors.displayName.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="git-access-token">
              Personal Access Token (PAT) / API Key
            </Label>
            <Input
              id="git-access-token"
              type="password"
              placeholder="ghp_••••••••••••••••••••••••••••••••••••"
              {...register('accessToken')}
            />
            <p className="text-[11px] text-muted-foreground">
              Encrypted at rest with AES-256-GCM. Requires repository read permissions.
            </p>
            {errors.accessToken && (
              <p className="text-xs text-destructive">{errors.accessToken.message}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="git-webhook-secret">Webhook Secret (Optional)</Label>
            <Input
              id="git-webhook-secret"
              type="password"
              placeholder="Secret for push event payload verification"
              {...register('webhookSecret')}
            />
            {errors.webhookSecret && (
              <p className="text-xs text-destructive">{errors.webhookSecret.message}</p>
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
              {submitting ? 'Connecting…' : 'Connect Provider'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
