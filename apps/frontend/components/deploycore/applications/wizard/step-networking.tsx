'use client'

import type { FieldErrors, UseFormRegister } from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { CreateApplicationValues } from '@/lib/validations/application'

interface StepNetworkingProps {
  register: UseFormRegister<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepNetworking({ register, errors }: StepNetworkingProps) {
  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.domain) || undefined}>
        <FieldLabel htmlFor="wizard-domain">Domain</FieldLabel>
        <Input
          id="wizard-domain"
          placeholder="api.example.com"
          className="font-mono"
          aria-invalid={Boolean(errors.domain)}
          {...register('domain')}
        />
        <FieldDescription>Optional. Leave blank to assign a domain later.</FieldDescription>
        <FieldError>{errors.domain?.message}</FieldError>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={Boolean(errors.healthCheckPath) || undefined}>
          <FieldLabel htmlFor="wizard-health-path">Health check path</FieldLabel>
          <Input
            id="wizard-health-path"
            className="font-mono"
            aria-invalid={Boolean(errors.healthCheckPath)}
            {...register('healthCheckPath')}
          />
          <FieldError>{errors.healthCheckPath?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.healthCheckPort) || undefined}>
          <FieldLabel htmlFor="wizard-health-port">Health check port</FieldLabel>
          <Input
            id="wizard-health-port"
            type="number"
            inputMode="numeric"
            aria-invalid={Boolean(errors.healthCheckPort)}
            {...register('healthCheckPort')}
          />
          <FieldError>{errors.healthCheckPort?.message}</FieldError>
        </Field>
      </div>
    </FieldGroup>
  )
}
