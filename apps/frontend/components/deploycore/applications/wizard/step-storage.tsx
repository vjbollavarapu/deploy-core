'use client'

import { Plus, Trash2 } from 'lucide-react'
import {
  Controller,
  useFieldArray,
  type Control,
  type FieldErrors,
  type UseFormRegister,
  type UseFormWatch,
} from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { CreateApplicationValues } from '@/lib/validations/application'

interface StepStorageProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  lockedNames?: readonly string[]
}

function isLocked(names: readonly string[], value: string): boolean {
  const key = value.trim().toLowerCase()
  if (!key) return false
  return names.some((name) => name.trim().toLowerCase() === key)
}

export function StepStorage({ register, control, watch, errors, lockedNames = [] }: StepStorageProps) {
  const volumes = useFieldArray({ control, name: 'volumes' })

  return (
    <FieldGroup>
      <div className="flex items-center justify-between gap-2">
        <div>
          <FieldLabel>Persistent storage</FieldLabel>
          <FieldDescription>
            Optional. Leave this empty for a stateless application. A volume is created and attached
            only when you deploy.
          </FieldDescription>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => volumes.append({ name: '', mountPath: '', writable: true })}
        >
          <Plus data-icon="inline-start" />
          Add volume
        </Button>
      </div>

      {volumes.fields.length === 0 ? (
        <p className="text-xs text-muted-foreground">No volumes. This application stays stateless.</p>
      ) : (
        <div className="flex flex-col gap-3">
          {volumes.fields.map((field, index) => {
            const locked = isLocked(lockedNames, watch(`volumes.${index}.name`) ?? '')
            return (
              <div key={field.id} className="grid gap-2 rounded-lg border border-border p-3">
                <div className="grid gap-2 sm:grid-cols-2">
                  <Field data-invalid={Boolean(errors.volumes?.[index]?.name) || undefined}>
                    <FieldLabel htmlFor={`volume-name-${index}`}>Volume name</FieldLabel>
                    <Input
                      id={`volume-name-${index}`}
                      placeholder="redis-data"
                      className="font-mono"
                      aria-invalid={Boolean(errors.volumes?.[index]?.name)}
                      readOnly={locked}
                      {...register(`volumes.${index}.name`)}
                    />
                    <FieldError>{errors.volumes?.[index]?.name?.message}</FieldError>
                  </Field>
                  <Field data-invalid={Boolean(errors.volumes?.[index]?.mountPath) || undefined}>
                    <FieldLabel htmlFor={`volume-path-${index}`}>Container mount path</FieldLabel>
                    <Input
                      id={`volume-path-${index}`}
                      placeholder="/data"
                      className="font-mono"
                      aria-invalid={Boolean(errors.volumes?.[index]?.mountPath)}
                      readOnly={locked}
                      {...register(`volumes.${index}.mountPath`)}
                    />
                    <FieldError>{errors.volumes?.[index]?.mountPath?.message}</FieldError>
                  </Field>
                </div>
                <div className="flex items-end gap-2">
                  <Field className="flex-1">
                    <FieldLabel>Access</FieldLabel>
                    <Controller
                      name={`volumes.${index}.writable`}
                      control={control}
                      render={({ field: access }) => (
                        <Select
                          value={access.value ? 'writable' : 'readonly'}
                          disabled={locked}
                          onValueChange={(value) => {
                            if (value) access.onChange(value === 'writable')
                          }}
                        >
                          <SelectTrigger className="w-full" aria-label={`Volume ${index + 1} access`}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="writable">Writable</SelectItem>
                            <SelectItem value="readonly">Read-only</SelectItem>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-8"
                    onClick={() => volumes.remove(index)}
                    disabled={locked}
                    aria-label={`Remove volume ${index + 1}`}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </FieldGroup>
  )
}
