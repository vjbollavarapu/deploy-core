'use client'

import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Plus,
  RefreshCw,
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

function formatTokenExpiry(expiresAt: string | null | undefined): string | null {
  if (!expiresAt) return null
  const d = new Date(expiresAt)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export function AddServerWizard({ onSuccess }: AddServerWizardProps = {}) {
  const { activeOrg } = useOrganization()
  const demo = isDemoModeEnabled()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [verified, setVerified] = useState(false)
  const [checking, setChecking] = useState(false)
  const [creating, setCreating] = useState(false)
  const [regenerating, setRegenerating] = useState(false)
  const [createdServerId, setCreatedServerId] = useState<string | null>(null)
  const [registrationToken, setRegistrationToken] = useState<string | null>(null)
  const [tokenExpiresAt, setTokenExpiresAt] = useState<string | null>(null)
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
  const expiryLabel = formatTokenExpiry(tokenExpiresAt)

  function resetWizardState() {
    setStep(0)
    setVerified(false)
    setChecking(false)
    setCreating(false)
    setRegenerating(false)
    setCreatedServerId(null)
    setRegistrationToken(null)
    setTokenExpiresAt(null)
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

  function applyIssuedToken(token: string, expiresAt?: string | null) {
    setRegistrationToken(token)
    setTokenExpiresAt(expiresAt ?? null)
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
      applyIssuedToken(
        REGISTRATION_TOKEN_PLACEHOLDER,
        new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      )
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
      applyIssuedToken(token, tok.registrationToken?.expiresAt)
      return true
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Failed to register server')
      return false
    } finally {
      setCreating(false)
    }
  }

  async function regenerateRegistrationToken() {
    if (regenerating || creating) return

    if (demo) {
      applyIssuedToken(
        REGISTRATION_TOKEN_PLACEHOLDER,
        new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      )
      toast.success('Issued a new demo registration token')
      return
    }

    if (!createdServerId) {
      toast.error('Create the server before issuing a registration token.')
      return
    }

    setRegenerating(true)
    try {
      const tok = await apiClient.post<RegistrationTokenResponse>(
        `/servers/${createdServerId}/registration-token`,
      )
      const token = tok.registrationToken?.token
      if (!token) {
        toast.error('Control Plane did not return a registration token.')
        return
      }
      applyIssuedToken(token, tok.registrationToken?.expiresAt)
      toast.success('New registration token issued')
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Failed to issue registration token')
    } finally {
      setRegenerating(false)
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
      <DialogContent className="flex max-h-[min(90vh,52rem)] w-full max-w-[calc(100%-2rem)] flex-col gap-4 overflow-hidden sm:max-w-5xl">
        <DialogHeader className="min-w-0 shrink-0">
          <DialogTitle>Add server</DialogTitle>
          <DialogDescription>
            Register a host, install the agent, and verify connectivity.
          </DialogDescription>
        </DialogHeader>

        <ol className="grid min-w-0 shrink-0 grid-cols-3 gap-2 sm:grid-cols-6" aria-label="Wizard progress">
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
                    'w-full truncate text-center text-[10px] leading-tight',
                    currentStep ? 'font-medium text-foreground' : 'text-muted-foreground',
                  )}
                >
                  {item.label}
                </span>
              </li>
            )
          })}
        </ol>

        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden pr-0.5">
          <div className="min-h-56 min-w-0">
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
            <div className="flex min-w-0 flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                A temporary registration token was issued for{' '}
                <span className="font-medium text-foreground">{values.name || 'this server'}</span>
                {createdServerId ? (
                  <>
                    {' '}
                    (<span className="break-all font-mono text-xs">{createdServerId}</span>)
                  </>
                ) : null}
                . Tokens are single-use and time-limited. If this one expires before the agent
                registers, generate a new token for the same server — do not delete or recreate
                the server.
              </p>
              <div className="min-w-0 rounded-lg border border-border bg-muted/40 px-3 py-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                    <Shield className="size-3.5 shrink-0" />
                    Temporary token
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    disabled={regenerating || creating || (!createdServerId && !demo)}
                    onClick={() => void regenerateRegistrationToken()}
                  >
                    <RefreshCw
                      data-icon="inline-start"
                      className={cn(regenerating && 'animate-spin')}
                    />
                    {regenerating ? 'Generating…' : 'Generate new token'}
                  </Button>
                </div>
                <p className="mt-2 break-all font-mono text-sm text-foreground">
                  {registrationToken || (demo ? REGISTRATION_TOKEN_PLACEHOLDER : 'Issuing…')}
                </p>
                {expiryLabel ? (
                  <p className="mt-1.5 text-xs text-muted-foreground">Expires {expiryLabel}</p>
                ) : (
                  <p className="mt-1.5 text-xs text-muted-foreground">
                    Temporary and single-use. Generate a new token if registration fails or the
                    current token expires.
                  </p>
                )}
              </div>
              <CodeBlock
                code={registrationCommand}
                label="Registration command"
                className="min-w-0 w-full"
              />
              <p className="text-xs text-muted-foreground">
                Copy the command with the current token. Do not commit this token to source
                control. After generating a new token, use the updated command only.
              </p>
            </div>
          )}

          {current.id === 'install' && (
            <div className="flex min-w-0 flex-col gap-3 text-sm">
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
              <CodeBlock
                code={registrationCommand}
                label="Install on host"
                className="min-w-0 w-full"
              />
              <p className="text-xs text-muted-foreground">
                If the token expires before first registration, go back to Registration and choose
                Generate new token for this same server.
              </p>
            </div>
          )}

          {current.id === 'verification' && (
            <div className="flex min-w-0 flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                Waiting for the first heartbeat from{' '}
                <span className="font-medium text-foreground">{values.name}</span>
                {createdServerId ? (
                  <>
                    {' '}
                    (<span className="break-all font-mono text-xs">{createdServerId}</span>)
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
                  <p className="break-all pl-4 text-xs opacity-80">{verifyDetail}</p>
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
        </div>

        <DialogFooter className="shrink-0 items-center sm:justify-between">
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
            <Button
              type="button"
              size="sm"
              disabled={creating || regenerating}
              onClick={() => void goNext()}
            >
              {creating ? 'Registering…' : 'Continue'}
              <ChevronRight data-icon="inline-end" />
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
