'use client'

import { Plus, Trash2 } from 'lucide-react'
import {
  useFieldArray,
  type Control,
  type FieldErrors,
  type UseFormRegister,
  type UseFormWatch,
} from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { isPersistedVariableKey } from '@/lib/applications/create-application-flow'
import type { CreateApplicationValues } from '@/lib/validations/application'

interface StepConfigurationProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  lockedKeys?: readonly string[]
}

export function StepConfiguration({
  register,
  control,
  watch,
  errors,
  lockedKeys = [],
}: StepConfigurationProps) {
  const envFields = useFieldArray({ control, name: 'envVars' })

  return (
    <FieldGroup>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <FieldLabel>Environment variables</FieldLabel>
            <FieldDescription>
              Plain configuration values injected at runtime.
              {lockedKeys.length > 0
                ? ' Saved variables stay as stored. Deploy retries only variables that are not saved yet.'
                : ''}
            </FieldDescription>
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
            {envFields.fields.map((field, index) => {
              const locked = isPersistedVariableKey(lockedKeys, watch(`envVars.${index}.key`) ?? '')
              return (
                <div key={field.id} className="grid grid-cols-[1fr_1fr_auto] gap-2">
                <Field data-invalid={Boolean(errors.envVars?.[index]?.key) || undefined}>
                  <Input
                    placeholder="KEY"
                    className="font-mono"
                    aria-label={`Environment variable ${index + 1} key`}
                    aria-invalid={Boolean(errors.envVars?.[index]?.key)}
                    {...register(`envVars.${index}.key`)}
                    readOnly={locked}
                  />
                  <FieldError>{errors.envVars?.[index]?.key?.message}</FieldError>
                </Field>
                <Field data-invalid={Boolean(errors.envVars?.[index]?.value) || undefined}>
                  <Input
                    placeholder="value"
                    className="font-mono"
                    aria-label={`Environment variable ${index + 1} value`}
                    {...register(`envVars.${index}.value`)}
                    readOnly={locked}
                  />
                </Field>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  onClick={() => envFields.remove(index)}
                  disabled={locked}
                  aria-label={`Remove environment variable ${index + 1}`}
                >
                  <Trash2 className="size-3.5" />
                </Button>
                </div>
              )
            })}
          </div>
        )}
        <FieldError>{typeof errors.envVars?.message === 'string' ? errors.envVars.message : null}</FieldError>
      </div>

      <div className="flex flex-col gap-2 border-t border-border pt-4">
        <FieldLabel>Secrets</FieldLabel>
        <FieldDescription>
          Secret values are not entered in this wizard and are not saved as environment variables.
          Existing secrets in the organization, project, environment, and application scopes are
          included when the revision is created. Add them in Security → Secrets before deploying
          if this application needs them.
        </FieldDescription>
      </div>
    </FieldGroup>
  )
}
