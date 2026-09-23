import { z } from 'zod'

export const SECRET_SCOPES = ['Organization', 'Project', 'Environment', 'Application'] as const
export type SecretScope = (typeof SECRET_SCOPES)[number]

export const ACCESS_ROLES = ['Owner', 'Admin', 'Developer', 'Viewer'] as const
export type AccessRole = (typeof ACCESS_ROLES)[number]

export const secretNameSchema = z
  .string()
  .trim()
  .min(1, 'Secret name is required')
  .regex(
    /^[A-Za-z_][A-Za-z0-9_]*$/,
    'Secret name must start with a letter or underscore and contain only alphanumeric characters and underscores',
  )

export const addSecretSchema = z.object({
  name: secretNameSchema,
  scope: z.enum(SECRET_SCOPES),
  applications: z.string().optional().default(''),
  description: z.string().optional().default(''),
  value: z
    .string()
    .trim()
    .min(1, 'Secret value is required (values are write-only and never shown)'),
  accessRoles: z.array(z.string()).default(['Owner', 'Admin']),
})

export type AddSecretFormValues = z.infer<typeof addSecretSchema>

export const rotateSecretSchema = z.object({
  value: z
    .string()
    .trim()
    .min(1, 'New secret value is required (current value is never shown)'),
})

export type RotateSecretFormValues = z.infer<typeof rotateSecretSchema>

export const scopedAccessSchema = z.object({
  accessRoles: z.array(z.string()).min(1, 'Select at least one role with access'),
})

export type ScopedAccessFormValues = z.infer<typeof scopedAccessSchema>
