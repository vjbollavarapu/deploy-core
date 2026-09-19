'use client'

import type { FieldErrors, UseFormRegister, UseFormSetValue, UseFormWatch } from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  sourceTypeLabel,
  type CreateApplicationValues,
  type SourceType,
} from '@/lib/validations/application'
import { Container, FileStack, GitBranch } from 'lucide-react'

const OPTIONS: { value: SourceType; icon: typeof GitBranch; description: string }[] = [
  {
    value: 'git',
    icon: GitBranch,
    description: 'Build from a Git repository with a Dockerfile.',
  },
  {
    value: 'docker-image',
    icon: Container,
    description: 'Deploy an existing image from a registry.',
  },
  {
    value: 'docker-compose',
    icon: FileStack,
    description: 'Deploy services defined in a Compose file.',
  },
]

interface StepSourceProps {
  setValue: UseFormSetValue<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepSource({ setValue, watch, errors }: StepSourceProps) {
  const selected = watch('sourceType')

  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.sourceType) || undefined}>
        <FieldLabel>Source</FieldLabel>
        <FieldDescription>Choose how this application will be built and deployed.</FieldDescription>
        <div className="grid gap-2">
          {OPTIONS.map((option) => {
            const Icon = option.icon
            const active = selected === option.value
            return (
              <button
                key={option.value}
                type="button"
                className={cn(
                  'flex items-start gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors',
                  active
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:border-primary/40 hover:bg-muted/30',
                )}
                onClick={() => setValue('sourceType', option.value, { shouldValidate: true })}
                aria-pressed={active}
              >
                <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{sourceTypeLabel(option.value)}</span>
                  <span className="block text-xs text-muted-foreground">{option.description}</span>
                </span>
              </button>
            )
          })}
        </div>
        <FieldError>{errors.sourceType?.message}</FieldError>
      </Field>
    </FieldGroup>
  )
}

interface StepSourceConfigProps {
  register: UseFormRegister<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepSourceConfig({ register, watch, errors }: StepSourceConfigProps) {
  const sourceType = watch('sourceType')

  return (
    <FieldGroup>
      {(sourceType === 'git' || sourceType === 'docker-compose') && (
        <>
          <Field data-invalid={Boolean(errors.repository) || undefined}>
            <FieldLabel htmlFor="wizard-repository">Repository</FieldLabel>
            <Input
              id="wizard-repository"
              placeholder="github.com/org/app"
              aria-invalid={Boolean(errors.repository)}
              {...register('repository')}
            />
            <FieldError>{errors.repository?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.branch) || undefined}>
            <FieldLabel htmlFor="wizard-branch">Branch</FieldLabel>
            <Input id="wizard-branch" aria-invalid={Boolean(errors.branch)} {...register('branch')} />
            <FieldError>{errors.branch?.message}</FieldError>
          </Field>
        </>
      )}

      {sourceType === 'git' && (
        <>
          <Field data-invalid={Boolean(errors.dockerfile) || undefined}>
            <FieldLabel htmlFor="wizard-dockerfile">Dockerfile</FieldLabel>
            <Input
              id="wizard-dockerfile"
              className="font-mono"
              aria-invalid={Boolean(errors.dockerfile)}
              {...register('dockerfile')}
            />
            <FieldError>{errors.dockerfile?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.buildContext) || undefined}>
            <FieldLabel htmlFor="wizard-build-context">Build context</FieldLabel>
            <Input
              id="wizard-build-context"
              className="font-mono"
              aria-invalid={Boolean(errors.buildContext)}
              {...register('buildContext')}
            />
            <FieldDescription>Path relative to the repository root.</FieldDescription>
            <FieldError>{errors.buildContext?.message}</FieldError>
          </Field>
        </>
      )}

      {sourceType === 'docker-image' && (
        <>
          <Field data-invalid={Boolean(errors.image) || undefined}>
            <FieldLabel htmlFor="wizard-image">Image</FieldLabel>
            <Input
              id="wizard-image"
              className="font-mono"
              placeholder="registry.example.com/app"
              aria-invalid={Boolean(errors.image)}
              {...register('image')}
            />
            <FieldError>{errors.image?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.imageTag) || undefined}>
            <FieldLabel htmlFor="wizard-image-tag">Tag</FieldLabel>
            <Input
              id="wizard-image-tag"
              className="font-mono"
              aria-invalid={Boolean(errors.imageTag)}
              {...register('imageTag')}
            />
            <FieldError>{errors.imageTag?.message}</FieldError>
          </Field>
        </>
      )}

      {sourceType === 'docker-compose' && (
        <Field data-invalid={Boolean(errors.composeFile) || undefined}>
          <FieldLabel htmlFor="wizard-compose">Compose file</FieldLabel>
          <Input
            id="wizard-compose"
            className="font-mono"
            aria-invalid={Boolean(errors.composeFile)}
            {...register('composeFile')}
          />
          <FieldError>{errors.composeFile?.message}</FieldError>
        </Field>
      )}
    </FieldGroup>
  )
}

export type WizardFieldErrors = FieldErrors<CreateApplicationValues>
