'use client'

import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Plus,
  Server as ServerIcon,
  Shield,
} from 'lucide-react'
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
import { CodeBlock } from '@/components/platform/code-block'
import { apiClient, ApiError, type Server as WireServer } from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import { cn } from '@/lib/utils'
import {
  buildRegistrationCommand,
  REGISTRATION_TOKEN_PLACEHOLDER,
  SERVER_PROVIDERS,
} from '@/lib/servers'
import {
  DEFAULT_ADD_SERVER_VALUES,
  SERVER_WIZARD_STEPS,
  STEP_SCHEMAS,
  type AddServerValues,
} from '@/lib/validations/server'

interface AddServerWizardProps {
  onSuccess?: () => void
}

type CreateServerResponse = { server?: WireServer }
type RegistrationTokenResponse = {
  registrationToken?: {
    serverId?: string
    agentId?: string
    token?: string
    expiresAt?: string
  }
}
type GetServerResponse = { server?: WireServer }

export function AddServerWizard({ onSuccess }: AddServerWizardProps = {}) {
  const { activeOrg } = useOrganization()
  const demo = isDemoModeEnabled()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [verified, setVerified] = useState(false)
  const [checking, setChecking] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createdServerId, setCreatedServerId] = useState<string | null>(null)
  const [registrationToken, setRegistrationToken] = useState<string | null>(null)
  const [verifyStatus, setVerifyStatus] = useState<string | null>(null)
  const [verifyDetail, setVerifyDetail] = useState<string | null>(null)

  const form = useForm<AddServerValues>({
    defaultValues: DEFAULT_ADD_SERVER_VALUES,
    mode: 'onSubmit',
    shouldUnregister: false,
  })

  const {
    register,
    reset,
    getValues,
    setValue,
    setError,
    clearErrors,
    control,
    formState: { errors },
  } = form

  const values = useWatch({ control })
  const tokenForCommand = registrationToken || (demo ? REGISTRATION_TOKEN_PLACEHOLDER : '…')
  const serverIdForCommand = createdServerId || (demo ? '<SERVER_UUID>' : '…')
  const registrationCommand = buildRegistrationCommand(tokenForCommand, serverIdForCommand)

  function resetWizardState() {
    setStep(0)
    setVerified(false)
    setChecking(false)
    setCreating(false)
    setCreatedServerId(null)
    setRegistrationToken(null)
    setVerifyStatus(null)
    setVerifyDetail(null)
    reset(DEFAULT_ADD_SERVER_VALUES)
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      resetWizardState()
    }
  }

  function validateCurrentStep(): boolean {
    clearErrors()
    const current = SERVER_WIZARD_STEPS[step]
    if (current.id === 'information') {
      const parsed = STEP_SCHEMAS.information.safeParse(getValues())
      if (!parsed.success) {
        for (const issue of parsed.error.issues) {
          const path = issue.path.join('.') as keyof AddServerValues
          if (path) setError(path, { type: 'manual', message: issue.message })
        }
        return false
      }
      return true
    }
    if (current.id === 'provider') {
      const parsed = STEP_SCHEMAS.provider.safeParse(getValues())
      if (!parsed.success) {
        for (const issue of parsed.error.issues) {
          const path = issue.path.join('.') as keyof AddServerValues
          if (path) setError(path, { type: 'manual', message: issue.message })
        }
        return false
      }
      return true
    }
    if (current.id === 'verification' && !verified) {
      toast.error('Verify the agent heartbeat before continuing')
      return false
    }
    return true
  }

  async function ensureServerRegistered(): Promise<boolean> {
    if (createdServerId && registrationToken) return true

    if (demo) {
      setCreatedServerId('demo-server')
      setRegistrationToken(REGISTRATION_TOKEN_PLACEHOLDER)
      return true
    }

    if (!activeOrg?.id) {
      toast.error('Select an organization before registering a server.')
      return false
    }

    const vals = getValues()
    setCreating(true)
    try {
      const created = await apiClient.post<CreateServerResponse>('/servers', {
        organizationId: activeOrg.id,
        name: vals.name,
        hostname: vals.name,
        provider: vals.provider,
        region: vals.region,
        ...(vals.publicIp ? { publicIp: vals.publicIp } : {}),
        labels: {
          ...(vals.labels ? { labels: vals.labels } : {}),
        },
      })
      const serverId = created.server?.id
      if (!serverId) {
        toast.error('Control Plane did not return a server id.')
        return false
      }

      const tok = await apiClient.post<RegistrationTokenResponse>(
        `/servers/${serverId}/registration-token`,
      )
      const token = tok.registrationToken?.token
      if (!token) {
        toast.error('Control Plane did not return a registration token.')
        return false
      }

      setCreatedServerId(serverId)
      setRegistrationToken(token)
      return true
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Failed to register server')
      return false
    } finally {
      setCreating(false)
    }
  }

  async function goNext() {
    if (!validateCurrentStep()) return
    const nextIndex = Math.min(SERVER_WIZARD_STEPS.length - 1, step + 1)
    const nextStep = SERVER_WIZARD_STEPS[nextIndex]
    if (nextStep.id === 'registration') {
      const ok = await ensureServerRegistered()
      if (!ok) return
    }
    setStep(nextIndex)
  }

  async function runVerification() {
    setChecking(true)
    setVerifyDetail(null)
    try {
      if (demo) {
        setVerified(true)
        setVerifyStatus('DEMO')
        setVerifyDetail('Demo mode does not query Control Plane heartbeats.')
        toast.message('Demo mode: heartbeat verification skipped')
        return
      }

      if (!createdServerId) {
        toast.error('Create the server before verifying the agent heartbeat.')
        setVerified(false)
        return
      }

      const res = await apiClient.get<GetServerResponse>(`/servers/${createdServerId}`)
      const server = res.server
      if (!server) {
        setVerified(false)
        setVerifyStatus(null)
        toast.error('Server not found in Control Plane.')
        return
      }

      const status = (server.status || 'OFFLINE').toUpperCase()
      const heartbeatAt = server.lastHeartbeatAt
      setVerifyStatus(status)

      const hasHeartbeat = Boolean(heartbeatAt)
      const agentPresent = status === 'ONLINE' || status === 'DEGRADED'

      if (hasHeartbeat && agentPresent) {
        setVerified(true)
        setVerifyDetail(`lastHeartbeatAt=${heartbeatAt}`)
        toast.success(`Agent heartbeat received (${status})`)
        return
      }

      setVerified(false)
      if (!hasHeartbeat) {
        setVerifyDetail(`Status ${status}. No lastHeartbeatAt yet — install and start the agent, then check again.`)
        toast.message(`Agent has not heartbeated yet (status: ${status})`)
      } else {
        setVerifyDetail(`lastHeartbeatAt present but status is ${status}`)
        toast.message(`Server status is ${status}; waiting for ONLINE or DEGRADED`)
      }
    } catch (err) {
      setVerified(false)
      toast.error(err instanceof ApiError ? err.message : 'Failed to check server heartbeat')
    } finally {
      setChecking(false)
    }
  }

  function finish() {
    const vals = getValues()
    if (!demo && !createdServerId) {
      toast.error('Server was not registered with the Control Plane.')
      return
    }
    toast.success(`${vals.name} registered`)
    handleOpenChange(false)
    onSuccess?.()
  }

  const isLast = step === SERVER_WIZARD_STEPS.length - 1
  const current = SERVER_WIZARD_STEPS[step]

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button size="sm">
            <Plus data-icon="inline-start" />
            Add server
          </Button>
        }
      />
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Add server</DialogTitle>
          <DialogDescription>
            Register a host, install the agent, and verify connectivity.
          </DialogDescription>
        </DialogHeader>

        <ol className="grid grid-cols-3 gap-2 sm:grid-cols-6" aria-label="Wizard progress">
          {SERVER_WIZARD_STEPS.map((item, index) => {
            const complete = index < step
            const currentStep = index === step
            return (
              <li key={item.id} className="flex min-w-0 flex-col items-center gap-1">
                <span
                  className={cn(
                    'flex size-6 items-center justify-center rounded-full text-[11px] font-medium',
                    complete && 'bg-primary text-primary-foreground',
                    currentStep && 'bg-primary/15 text-primary ring-1 ring-primary/40',
                    !complete && !currentStep && 'bg-muted text-muted-foreground',
                  )}
                  aria-current={currentStep ? 'step' : undefined}
                >
                  {complete ? <Check className="size-3.5" aria-hidden /> : index + 1}
                </span>
                <span
                  className={cn(
                    'truncate text-center text-[10px] leading-tight',
                    currentStep ? 'font-medium text-foreground' : 'text-muted-foreground',
                  )}
                >
                  {item.label}
                </span>
              </li>
            )
          })}
        </ol>

        <div className="min-h-56">
          {current.id === 'information' && (
            <FieldGroup>
              <Field data-invalid={Boolean(errors.name)}>
                <FieldLabel htmlFor="srv-name">Server name</FieldLabel>
                <Input
                  id="srv-name"
                  placeholder="hetzner-fsn1-03"
                  aria-invalid={Boolean(errors.name)}
                  {...register('name')}
                />
                <FieldDescription>Lowercase hostname used across the control plane.</FieldDescription>
                {errors.name ? <FieldError>{errors.name.message}</FieldError> : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-labels">Labels (optional)</FieldLabel>
                <Input
                  id="srv-labels"
                  placeholder="env=production, tier=edge"
                  {...register('labels')}
                />
              </Field>
            </FieldGroup>
          )}

          {current.id === 'provider' && (
            <FieldGroup>
              <Field data-invalid={Boolean(errors.provider)}>
                <FieldLabel htmlFor="srv-provider">Provider</FieldLabel>
                <Select
                  value={values.provider}
                  onValueChange={(v) =>
                    setValue('provider', (v as AddServerValues['provider']) ?? 'Hetzner', {
                      shouldValidate: true,
                    })
                  }
                >
                  <SelectTrigger id="srv-provider" className="w-full" aria-invalid={Boolean(errors.provider)}>
                    <SelectValue placeholder="Select a provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {SERVER_PROVIDERS.map((provider) => (
                      <SelectItem key={provider} value={provider}>
                        {provider}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {errors.provider ? <FieldError>{errors.provider.message}</FieldError> : null}
              </Field>
              <Field data-invalid={Boolean(errors.region)}>
                <FieldLabel htmlFor="srv-region">Region</FieldLabel>
                <Input
                  id="srv-region"
                  placeholder="eu-central-1"
                  aria-invalid={Boolean(errors.region)}
                  {...register('region')}
                />
                {errors.region ? <FieldError>{errors.region.message}</FieldError> : null}
              </Field>
              <Field data-invalid={Boolean(errors.publicIp)}>
                <FieldLabel htmlFor="srv-ip">Public IP (optional)</FieldLabel>
                <Input
                  id="srv-ip"
                  placeholder="203.0.113.50"
                  aria-invalid={Boolean(errors.publicIp)}
                  {...register('publicIp')}
                />
                <FieldDescription>
                  Leave blank to detect automatically once the agent connects.
                </FieldDescription>
                {errors.publicIp ? <FieldError>{errors.publicIp.message}</FieldError> : null}
              </Field>
            </FieldGroup>
          )}

          {current.id === 'registration' && (
            <div className="flex flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                A secure temporary registration token was issued for{' '}
                <span className="font-medium text-foreground">{values.name || 'this server'}</span>
                {createdServerId ? (
                  <>
                    {' '}
                    (<span className="font-mono text-xs">{createdServerId}</span>)
                  </>
                ) : null}
                . It expires shortly and can only be used once.
              </p>
              <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Shield className="size-3.5" />
                  Temporary token
                </div>
                <p className="mt-1 font-mono text-sm text-foreground break-all">
                  {registrationToken || (demo ? REGISTRATION_TOKEN_PLACEHOLDER : 'Issuing…')}
                </p>
              </div>
              <CodeBlock code={registrationCommand} label="Registration command" />
              <p className="text-xs text-muted-foreground">
                Copy the command with the token embedded. Do not commit this token to source control.
              </p>
            </div>
          )}

          {current.id === 'install' && (
            <div className="flex flex-col gap-3 text-sm">
              <p className="text-muted-foreground">
                On <span className="font-medium text-foreground">{values.name}</span>, run the
                registration command as root (or with sudo). The installer will:
              </p>
              <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
                <li>Install the DeployCore node agent package</li>
                <li>Register using the temporary token</li>
                <li>Start the agent systemd service</li>
                <li>Open an outbound mTLS connection to the control plane</li>
              </ul>
              <CodeBlock code={registrationCommand} label="Install on host" />
            </div>
          )}

          {current.id === 'verification' && (
            <div className="flex flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                Waiting for the first heartbeat from{' '}
                <span className="font-medium text-foreground">{values.name}</span>
                {createdServerId ? (
                  <>
                    {' '}
                    (<span className="font-mono text-xs">{createdServerId}</span>)
                  </>
                ) : null}
                .
              </p>
              <div
                className={cn(
                  'flex flex-col gap-1 rounded-lg border px-3 py-2.5 text-sm',
                  verified
                    ? 'border-success/30 bg-success/10 text-success'
                    : 'border-border bg-secondary/40 text-muted-foreground',
                )}
              >
                <div className="flex items-center gap-2">
                  <span className="relative flex size-2">
                    {!verified ? (
                      <>
                        <span className="absolute inline-flex size-full animate-ping rounded-full bg-warning/60" />
                        <span className="relative inline-flex size-2 rounded-full bg-warning" />
                      </>
                    ) : (
                      <span className="relative inline-flex size-2 rounded-full bg-success" />
                    )}
                  </span>
                  {verified
                    ? `Agent connected · heartbeat OK${verifyStatus ? ` (${verifyStatus})` : ''}`
                    : verifyStatus
                      ? `Waiting · Control Plane status ${verifyStatus}`
                      : 'Waiting for agent heartbeat…'}
                </div>
                {verifyDetail ? (
                  <p className="pl-4 text-xs opacity-80">{verifyDetail}</p>
                ) : null}
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                disabled={checking || verified}
                onClick={() => void runVerification()}
              >
                {checking ? 'Checking…' : verified ? 'Verified' : 'Check connection'}
              </Button>
            </div>
          )}

          {current.id === 'complete' && (
            <div className="flex flex-col items-start gap-3 py-2">
              <div className="flex size-10 items-center justify-center rounded-full bg-success/15 text-success">
                <Check className="size-5" />
              </div>
              <div>
                <p className="text-sm font-medium text-foreground">{values.name} is ready</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {values.provider} · {values.region}
                  {values.publicIp ? ` · ${values.publicIp}` : ''}
                </p>
              </div>
              <p className="text-sm text-muted-foreground">
                You can place applications on this host and manage maintenance mode from server
                settings.
              </p>
            </div>
          )}
        </div>

        <DialogFooter className="items-center sm:justify-between">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setStep((s) => Math.max(0, s - 1))}
            className={cn(step === 0 && 'invisible')}
          >
            <ChevronLeft data-icon="inline-start" />
            Back
          </Button>
          {isLast ? (
            <Button type="button" size="sm" onClick={finish}>
              <ServerIcon data-icon="inline-start" />
              Done
            </Button>
          ) : (
            <Button type="button" size="sm" disabled={creating} onClick={() => void goNext()}>
              {creating ? 'Registering…' : 'Continue'}
              <ChevronRight data-icon="inline-end" />
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
