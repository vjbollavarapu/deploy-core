'use client'

import { useState } from 'react'
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
import { projects } from '@/lib/mock-data'
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
import { StepPlacement } from './wizard/step-placement'
import { StepDeploy, StepReview } from './wizard/step-review'

const REVIEW_STEP = WIZARD_STEPS.findIndex((step) => step.id === 'review')
const DEPLOY_STEP = WIZARD_STEPS.findIndex((step) => step.id === 'deploy')

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

export function CreateApplicationWizard() {
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [serverError, setServerError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  const form = useForm<CreateApplicationValues>({
    defaultValues: DEFAULT_APPLICATION_VALUES,
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

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setStep(0)
      setServerError(null)
      setPending(false)
      reset(DEFAULT_APPLICATION_VALUES)
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
        const project = projects.find((item) => item.id === getValues('projectId'))
        const environment = getValues('environment')
        if (project && environment && !project.environments.includes(environment)) {
          setValue('environment', '')
          setError('environment', { type: 'manual', message: 'Select an environment' })
          return false
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

    setPending(true)
    try {
      await new Promise((resolve) => setTimeout(resolve, 700))
      toast.success(`Deployment started for ${parsed.data.name}`)
      handleOpenChange(false)
    } catch {
      setServerError('Unable to start deployment. Please try again.')
    } finally {
      setPending(false)
    }
  }

  const values = getValues()

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button size="sm">
            <Plus data-icon="inline-start" />
            New application
          </Button>
        }
      />
      <DialogContent className="flex max-h-[90vh] flex-col gap-4 overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Create application</DialogTitle>
          <DialogDescription>
            Configure source, runtime, networking, and placement. Nothing is submitted until Deploy.
          </DialogDescription>
        </DialogHeader>

        <WizardStepper step={step} />

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
            />
          )}
          {step === REVIEW_STEP && <StepReview values={values} />}
          {step === DEPLOY_STEP && <StepDeploy values={values} pending={pending} />}
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
