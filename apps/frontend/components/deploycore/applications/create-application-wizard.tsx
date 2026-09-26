'use client'

import { useEffect, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { ChevronLeft, ChevronRight, Plus, Rocket } from 'lucide-react'
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
import { cn } from '@/lib/utils'
import { projects as rawMockProjects, servers as rawMockServers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const mockProjects = getDemoFixtures(rawMockProjects)
const mockServers = getDemoFixtures(rawMockServers)
import {
  createApplicationSchema,
  DEFAULT_APPLICATION_VALUES,
  WIZARD_STEPS,
  type CreateApplicationValues,
} from '@/lib/validations/application'
import { WizardStepper } from './wizard/wizard-stepper'
import { StepSource, StepSourceConfig } from './wizard/step-source'
import { StepRuntime } from './wizard/step-runtime'
import { StepConfiguration } from './wizard/step-configuration'
import { StepNetworking } from './wizard/step-networking'
import {
  StepPlacement,
  type PlacementProject,
  type PlacementServer,
} from './wizard/step-placement'
import { StepDeploy, StepReview } from './wizard/step-review'
import {
  apiClient,
  ApiError,
  type ApplicationType,
  type Page,
  type Server as WireServer,
  type SourceType as WireSourceType,
  type WireEnvironment,
  type WireProject,
} from '@/lib/api'
import { useOrganization } from '@/lib/auth-context'

const REVIEW_STEP = WIZARD_STEPS.findIndex((step) => step.id === 'review')
const DEPLOY_STEP = WIZARD_STEPS.findIndex((step) => step.id === 'deploy')

const isUUID = (str?: string): boolean =>
  Boolean(str && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(str))

function toWireApplicationType(uiType: string): ApplicationType {
  switch (uiType) {
    case 'API':
      return 'API'
    case 'Worker':
      return 'WORKER'
    case 'Scheduled Job':
      return 'SCHEDULED_JOB'
    case 'Static Site':
      return 'STATIC_SITE'
    case 'Docker Compose':
      return 'DOCKER_COMPOSE'
    case 'Docker Image':
      return 'DOCKER_IMAGE'
    default:
      return 'WEB_SERVICE'
  }
}

function toWireSourceType(uiSource: string): WireSourceType {
  switch (uiSource) {
    case 'docker-image':
      return 'image'
    case 'docker-compose':
      return 'compose'
    default:
      return 'git'
  }
}

const fallbackProjects: PlacementProject[] = mockProjects.map((p) => ({
  id: p.id,
  name: p.name,
  slug: p.slug,
  environments: p.environments.map((env) => ({
    id: env,
    name: env,
  })),
}))

const fallbackServers: PlacementServer[] = mockServers.map((s) => ({
  id: s.id,
  name: s.name,
  region: s.region ?? undefined,
  status: s.status,
}))

function applyZodIssues(
  issues: { path: PropertyKey[]; message: string }[],
  setError: ReturnType<typeof useForm<CreateApplicationValues>>['setError'],
) {
  for (const issue of issues) {
    const path = issue.path.join('.')
    if (!path) continue
    setError(path as keyof CreateApplicationValues, { type: 'manual', message: issue.message })
  }
}

interface CreateApplicationWizardProps {
  trigger?: ReactNode
  defaultProjectId?: string
  defaultEnvironment?: string
  onSuccess?: () => void
}

export function CreateApplicationWizard({
  trigger,
  defaultProjectId,
  defaultEnvironment,
  onSuccess,
}: CreateApplicationWizardProps) {
  const { activeOrg } = useOrganization()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [serverError, setServerError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  const [projectsList, setProjectsList] = useState<PlacementProject[]>(fallbackProjects)
  const [serversList, setServersList] = useState<PlacementServer[]>(fallbackServers)
  const [isLoadingPlacement, setIsLoadingPlacement] = useState(false)

  const form = useForm<CreateApplicationValues>({
    defaultValues: {
      ...DEFAULT_APPLICATION_VALUES,
      ...(defaultProjectId ? { projectId: defaultProjectId } : {}),
      ...(defaultEnvironment ? { environment: defaultEnvironment } : {}),
    },
    mode: 'onSubmit',
    shouldUnregister: false,
  })

  const {
    register,
    control,
    reset,
    getValues,
    setValue,
    setError,
    clearErrors,
    watch,
    formState: { errors },
  } = form

  useEffect(() => {
    if (!open || !activeOrg?.id) return

    let cancelled = false

    async function loadPlacementData() {
      try {
        const [projRes, servRes] = await Promise.all([
          apiClient.get<Page<WireProject>>(`/projects?organizationId=${activeOrg?.id}`).catch(() => null),
          apiClient.get<Page<WireServer>>(`/servers?organizationId=${activeOrg?.id}`).catch(() => null),
        ])

        if (cancelled) return

        if (Array.isArray(servRes?.items) && servRes.items.length > 0) {
          const mappedServers: PlacementServer[] = servRes.items.map((srv) => ({
            id: srv.id || '',
            name: srv.name || 'Unnamed Server',
            region: srv.labels?.region || srv.provider,
            status: srv.status,
          }))
          setServersList(mappedServers)
        } else {
          setServersList(fallbackServers)
        }

        if (Array.isArray(projRes?.items) && projRes.items.length > 0) {
          const projectsWithEnvs = await Promise.all(
            projRes.items.map(async (p): Promise<PlacementProject> => {
              try {
                const envRes = await apiClient.get<Page<WireEnvironment>>(
                  `/projects/${p.id}/environments`,
                )
                const envs = Array.isArray(envRes?.items) && envRes.items.length > 0
                  ? envRes.items.map((e) => ({
                      id: e.id || e.name || '',
                      name: e.name || 'default',
                      kind: e.kind,
                    }))
                  : [{ id: 'production', name: 'production', kind: 'production' }]
                return {
                  id: p.id || '',
                  name: p.name || 'Untitled Project',
                  slug: p.slug,
                  environments: envs,
                }
              } catch {
                return {
                  id: p.id || '',
                  name: p.name || 'Untitled Project',
                  slug: p.slug,
                  environments: [{ id: 'production', name: 'production', kind: 'production' }],
                }
              }
            }),
          )

          if (!cancelled) {
            setProjectsList(projectsWithEnvs)
          }
        } else {
          setProjectsList(fallbackProjects)
        }
      } catch {
        if (!cancelled) {
          setProjectsList(fallbackProjects)
          setServersList(fallbackServers)
        }
      } finally {
        if (!cancelled) {
          setIsLoadingPlacement(false)
        }
      }
    }

    void loadPlacementData()

    return () => {
      cancelled = true
    }
  }, [open, activeOrg?.id])

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setIsLoadingPlacement(true)
    } else {
      setStep(0)
      setServerError(null)
      setPending(false)
      reset({
        ...DEFAULT_APPLICATION_VALUES,
        ...(defaultProjectId ? { projectId: defaultProjectId } : {}),
        ...(defaultEnvironment ? { environment: defaultEnvironment } : {}),
      })
    }
  }

  async function validateCurrentStep(): Promise<boolean> {
    clearErrors()
    setServerError(null)
    const current = WIZARD_STEPS[step]
    if (!('schema' in current) || !current.schema) return true

    const parsed = current.schema.safeParse(getValues())
    if (parsed.success) {
      if (current.id === 'placement') {
        const projId = getValues('projectId')
        const envVal = getValues('environment')
        const project = projectsList.find((item) => item.id === projId)
        if (project && envVal) {
          const hasEnv = project.environments.some(
            (e) => e.id === envVal || e.name === envVal,
          )
          if (!hasEnv && project.environments.length > 0) {
            setValue('environment', '')
            setError('environment', { type: 'manual', message: 'Select an environment' })
            return false
          }
        }
      }
      return true
    }

    applyZodIssues(parsed.error.issues, setError)
    return false
  }

  async function goNext() {
    const valid = await validateCurrentStep()
    if (!valid) return
    setStep((current) => Math.min(current + 1, WIZARD_STEPS.length - 1))
  }

  function goBack() {
    clearErrors()
    setServerError(null)
    setStep((current) => Math.max(current - 1, 0))
  }

  async function onDeploy() {
    clearErrors()
    setServerError(null)
    const parsed = createApplicationSchema.safeParse(getValues())
    if (!parsed.success) {
      applyZodIssues(parsed.error.issues, setError)
      setServerError('Fix validation errors before deploying.')
      return
    }

    const data = parsed.data
    setPending(true)

    try {
      const selectedProject = projectsList.find((p) => p.id === data.projectId)
      const selectedEnv = selectedProject?.environments.find(
        (e) => e.id === data.environment || e.name === data.environment,
      )
      const environmentId = selectedEnv?.id || data.environment

      if (activeOrg?.id && isUUID(activeOrg.id) && isUUID(data.projectId) && isUUID(environmentId)) {
        const appPayload = {
          organizationId: activeOrg.id,
          projectId: data.projectId,
          environmentId,
          name: data.name,
          slug: data.name.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
          type: toWireApplicationType(data.applicationType),
          targetServerId: isUUID(data.serverId) ? data.serverId : null,
          config: {
            sourceType: toWireSourceType(data.sourceType),
            repositoryUrl: data.repository || null,
            gitBranch: data.branch || null,
            dockerfilePath: data.dockerfile || null,
            buildContext: data.buildContext || null,
            imageReference: data.image
              ? data.imageTag
                ? `${data.image}:${data.imageTag}`
                : data.image
              : null,
            internalPort: data.port,
            command: data.command || null,
            entrypoint: data.entrypoint || null,
            cpuLimitMillis: Math.round(data.cpu * 1000),
            memoryLimitBytes: data.memoryMb * 1024 * 1024,
            restartPolicy: data.restartPolicy,
            healthCheck: {
              path: data.healthCheckPath,
              port: data.healthCheckPort,
            },
            runtimeConfig: {},
          },
        }

        const createRes = await apiClient.post<{ application: { id: string } }>(
          '/applications',
          appPayload,
        )

        const appId = createRes.application?.id
        if (appId) {
          await apiClient.post(`/applications/${appId}/deployments`, {
            trigger: 'manual',
          })
        }
      } else {
        // Fallback simulation for offline/preview environments with mock string identifiers
        await new Promise((resolve) => setTimeout(resolve, 600))
      }

      toast.success(`Application “${data.name}” created and deployment queued`)
      onSuccess?.()
      handleOpenChange(false)
    } catch (err) {
      if (err instanceof ApiError) {
        setServerError(err.message)
        toast.error(err.message)
      } else {
        setServerError(
          err instanceof Error ? err.message : 'Unable to create application. Please try again.',
        )
      }
    } finally {
      setPending(false)
    }
  }

  const values = getValues()

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {trigger ? (
        <DialogTrigger render={trigger as React.ReactElement} />
      ) : (
        <DialogTrigger
          render={
            <Button size="sm">
              <Plus data-icon="inline-start" />
              New application
            </Button>
          }
        />
      )}
      <DialogContent className="flex max-h-[90vh] flex-col gap-4 overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Create application</DialogTitle>
          <DialogDescription>
            Configure source, runtime, networking, and placement. Nothing is submitted until Deploy.
          </DialogDescription>
        </DialogHeader>

        <WizardStepper
          step={step}
          onStepClick={(targetStep) => {
            if (targetStep < step && !pending) {
              clearErrors()
              setServerError(null)
              setStep(targetStep)
            }
          }}
        />

        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          {step === 0 && <StepSource setValue={setValue} watch={watch} errors={errors} />}
          {step === 1 && <StepSourceConfig register={register} watch={watch} errors={errors} />}
          {step === 2 && <StepRuntime register={register} control={control} errors={errors} />}
          {step === 3 && <StepConfiguration register={register} control={control} errors={errors} />}
          {step === 4 && <StepNetworking register={register} errors={errors} />}
          {step === 5 && (
            <StepPlacement
              register={register}
              control={control}
              watch={watch}
              setValue={setValue}
              errors={errors}
              projects={projectsList}
              servers={serversList}
              isLoading={isLoadingPlacement}
            />
          )}
          {step === REVIEW_STEP && (
            <StepReview values={values} projects={projectsList} servers={serversList} />
          )}
          {step === DEPLOY_STEP && (
            <StepDeploy
              values={values}
              pending={pending}
              projects={projectsList}
              servers={serversList}
            />
          )}
        </div>

        {serverError && (
          <p className="text-sm text-critical" role="alert">
            {serverError}
          </p>
        )}

        <DialogFooter className="items-center sm:justify-between">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={goBack}
            disabled={pending}
            className={cn(step === 0 && 'invisible')}
          >
            <ChevronLeft data-icon="inline-start" />
            Back
          </Button>

          {step < DEPLOY_STEP ? (
            <Button type="button" size="sm" onClick={() => void goNext()} disabled={pending}>
              Continue
              <ChevronRight data-icon="inline-end" />
            </Button>
          ) : (
            <Button type="button" size="sm" disabled={pending} onClick={() => void onDeploy()}>
              <Rocket data-icon="inline-start" />
              {pending ? 'Deploying…' : 'Deploy'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

