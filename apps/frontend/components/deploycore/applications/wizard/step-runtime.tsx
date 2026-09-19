'use client'

import { Controller, type Control, type FieldErrors, type UseFormRegister } from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  APPLICATION_TYPES,
  RESTART_POLICIES,
  type CreateApplicationValues,
} from '@/lib/validations/application'

interface StepRuntimeProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepRuntime({ register, control, errors }: StepRuntimeProps) {
  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.applicationType) || undefined}>
        <FieldLabel htmlFor="wizard-app-type">Application type</FieldLabel>
        <Controller
          name="applicationType"
          control={control}
          render={({ field }) => (
            <Select
              value={field.value}
              onValueChange={(value) => {
                if (value) field.onChange(value)
              }}
            >
              <SelectTrigger id="wizard-app-type" className="w-full" aria-invalid={Boolean(errors.applicationType)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {APPLICATION_TYPES.map((type) => (
                  <SelectItem key={type} value={type}>
                    {type}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        <FieldError>{errors.applicationType?.message}</FieldError>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={Boolean(errors.port) || undefined}>
          <FieldLabel htmlFor="wizard-port">Internal port</FieldLabel>
          <Input
            id="wizard-port"
            type="number"
            inputMode="numeric"
            aria-invalid={Boolean(errors.port)}
            {...register('port')}
          />
          <FieldError>{errors.port?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.restartPolicy) || undefined}>
          <FieldLabel htmlFor="wizard-restart">Restart policy</FieldLabel>
          <Controller
            name="restartPolicy"
            control={control}
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value) => {
                  if (value) field.onChange(value)
                }}
              >
                <SelectTrigger id="wizard-restart" className="w-full" aria-invalid={Boolean(errors.restartPolicy)}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RESTART_POLICIES.map((policy) => (
                    <SelectItem key={policy} value={policy}>
                      {policy}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
          <FieldError>{errors.restartPolicy?.message}</FieldError>
        </Field>
      </div>

      <Field data-invalid={Boolean(errors.command) || undefined}>
        <FieldLabel htmlFor="wizard-command">Command</FieldLabel>
        <Input
          id="wizard-command"
          className="font-mono"
          placeholder="Optional override"
          aria-invalid={Boolean(errors.command)}
          {...register('command')}
        />
        <FieldDescription>Leave blank to use the image default CMD.</FieldDescription>
        <FieldError>{errors.command?.message}</FieldError>
      </Field>

      <Field data-invalid={Boolean(errors.entrypoint) || undefined}>
        <FieldLabel htmlFor="wizard-entrypoint">Entrypoint</FieldLabel>
        <Input
          id="wizard-entrypoint"
          className="font-mono"
          placeholder="Optional override"
          aria-invalid={Boolean(errors.entrypoint)}
          {...register('entrypoint')}
        />
        <FieldError>{errors.entrypoint?.message}</FieldError>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={Boolean(errors.cpu) || undefined}>
          <FieldLabel htmlFor="wizard-cpu">CPU (cores)</FieldLabel>
          <Input
            id="wizard-cpu"
            type="number"
            step="0.1"
            inputMode="decimal"
            aria-invalid={Boolean(errors.cpu)}
            {...register('cpu')}
          />
          <FieldError>{errors.cpu?.message}</FieldError>
        </Field>
        <Field data-invalid={Boolean(errors.memoryMb) || undefined}>
          <FieldLabel htmlFor="wizard-ram">RAM (MiB)</FieldLabel>
          <Input
            id="wizard-ram"
            type="number"
            inputMode="numeric"
            aria-invalid={Boolean(errors.memoryMb)}
            {...register('memoryMb')}
          />
          <FieldError>{errors.memoryMb?.message}</FieldError>
        </Field>
      </div>
    </FieldGroup>
  )
}
