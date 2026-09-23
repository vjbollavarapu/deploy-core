import { z } from 'zod'

export const ENV_SCOPES = ['Organization', 'Project', 'Environment', 'Application'] as const
export type EnvScope = (typeof ENV_SCOPES)[number]

export const variableKeySchema = z
  .string()
  .trim()
  .min(1, 'Variable key is required')
  .regex(
    /^[A-Za-z_][A-Za-z0-9_]*$/,
    'Key must start with a letter or underscore and contain only alphanumeric characters and underscores',
  )

export const addVariableSchema = z
  .object({
    key: variableKeySchema,
    value: z.string(),
    scope: z.enum(ENV_SCOPES),
    source: z.string().trim().min(1, 'Source is required'),
    secret: z.boolean().default(false),
  })
  .refine(
    (data) => {
      // Non-secret variables must have a value
      if (!data.secret && data.value.trim() === '') {
        return false
      }
      return true
    },
    {
      message: 'Value is required',
      path: ['value'],
    },
  )

export type AddVariableFormValues = z.infer<typeof addVariableSchema>

export const bulkPasteSchema = z.object({
  content: z.string().trim().min(1, 'Enter KEY=value lines to import'),
})

export type BulkPasteFormValues = z.infer<typeof bulkPasteSchema>
