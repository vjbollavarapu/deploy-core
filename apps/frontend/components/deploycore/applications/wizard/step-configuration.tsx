'use client'

import { Plus, Trash2 } from 'lucide-react'
import { useFieldArray, type Control, type FieldErrors, type UseFormRegister } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { CreateApplicationValues } from '@/lib/validations/application'

interface StepConfigurationProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
}

export function StepConfiguration({ register, control, errors }: StepConfigurationProps) {
  const envFields = useFieldArray({ control, name: 'envVars' })
  const secretFields = useFieldArray({ control, name: 'secrets' })

  return (
    <FieldGroup>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <FieldLabel>Environment variables</FieldLabel>
            <FieldDescription>Plain configuration values injected at runtime.</FieldDescription>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => envFields.append({ key: '', value: '' })}
          >
            <Plus data-icon="inline-start" />
            Add
          </Button>
        </div>
        {envFields.fields.length === 0 ? (
          <p className="text-xs text-muted-foreground">No environment variables added.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {envFields.fields.map((field, index) => (
              <div key={field.id} className="grid grid-cols-[1fr_1fr_auto] gap-2">
                <Field data-invalid={Boolean(errors.envVars?.[index]?.key) || undefined}>
                  <Input
                    placeholder="KEY"
                    className="font-mono"
                    aria-label={`Environment variable ${index + 1} key`}
                    aria-invalid={Boolean(errors.envVars?.[index]?.key)}
                    {...register(`envVars.${index}.key`)}
                  />
                  <FieldError>{errors.envVars?.[index]?.key?.message}</FieldError>
                </Field>
                <Field data-invalid={Boolean(errors.envVars?.[index]?.value) || undefined}>
                  <Input
                    placeholder="value"
                    className="font-mono"
                    aria-label={`Environment variable ${index + 1} value`}
                    {...register(`envVars.${index}.value`)}
                  />
                </Field>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  onClick={() => envFields.remove(index)}
                  aria-label={`Remove environment variable ${index + 1}`}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
        <FieldError>{typeof errors.envVars?.message === 'string' ? errors.envVars.message : null}</FieldError>
      </div>

      <div className="flex flex-col gap-3 border-t border-border pt-4">
        <div className="flex items-center justify-between gap-2">
          <div>
            <FieldLabel>Secrets</FieldLabel>
            <FieldDescription>Reference existing secrets by name. Values are never entered here.</FieldDescription>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => secretFields.append({ name: '' })}
          >
            <Plus data-icon="inline-start" />
            Add
          </Button>
        </div>
        {secretFields.fields.length === 0 ? (
          <p className="text-xs text-muted-foreground">No secret references added.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {secretFields.fields.map((field, index) => (
              <div key={field.id} className="grid grid-cols-[1fr_auto] gap-2">
                <Field data-invalid={Boolean(errors.secrets?.[index]?.name) || undefined}>
                  <Input
                    placeholder="SECRET_NAME"
                    className="font-mono"
                    aria-label={`Secret ${index + 1} name`}
                    aria-invalid={Boolean(errors.secrets?.[index]?.name)}
                    {...register(`secrets.${index}.name`)}
                  />
                  <FieldError>{errors.secrets?.[index]?.name?.message}</FieldError>
                </Field>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  onClick={() => secretFields.remove(index)}
                  aria-label={`Remove secret ${index + 1}`}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </FieldGroup>
  )
}
