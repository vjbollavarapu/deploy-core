import type { ReactNode } from 'react'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldSet } from '@/components/ui/field'
import { cn } from '@/lib/utils'

interface FormSectionProps {
  title?: string
  description?: string
  children: ReactNode
  className?: string
}

/** Standard form section spacing for console forms. */
export function FormSection({ title, description, children, className }: FormSectionProps) {
  return (
    <FieldSet className={cn('gap-4', className)}>
      {(title || description) && (
        <div className="flex flex-col gap-1">
          {title && <legend className="text-sm font-medium text-foreground">{title}</legend>}
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
        </div>
      )}
      <FieldGroup>{children}</FieldGroup>
    </FieldSet>
  )
}

export { Field, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldSet }
