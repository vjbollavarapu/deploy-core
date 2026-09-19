import { z } from 'zod'
import { SERVER_PROVIDERS } from '@/lib/servers'

export const SERVER_WIZARD_STEPS = [
  { id: 'information', label: 'Information' },
  { id: 'provider', label: 'Provider' },
  { id: 'registration', label: 'Registration' },
  { id: 'install', label: 'Install Agent' },
  { id: 'verification', label: 'Verification' },
  { id: 'complete', label: 'Complete' },
] as const

export const informationSchema = z.object({
  name: z
    .string()
    .trim()
    .min(2, 'Enter a server name')
    .max(64, 'Name is too long')
    .regex(/^[a-z0-9][a-z0-9-]*$/, 'Use lowercase letters, numbers, and hyphens'),
  labels: z.string().optional(),
})

export const providerSchema = z.object({
  provider: z.enum(SERVER_PROVIDERS, { message: 'Select a provider' }),
  region: z.string().trim().min(2, 'Enter a region'),
  publicIp: z
    .string()
    .trim()
    .optional()
    .refine(
      (value) => !value || /^(\d{1,3}\.){3}\d{1,3}$/.test(value) || value.includes(':'),
      'Enter a valid IPv4 or IPv6 address',
    ),
})

export const addServerSchema = informationSchema.merge(providerSchema)

export type AddServerValues = z.infer<typeof addServerSchema>

export const DEFAULT_ADD_SERVER_VALUES: AddServerValues = {
  name: '',
  labels: '',
  provider: 'Hetzner',
  region: '',
  publicIp: '',
}

export const STEP_SCHEMAS = {
  information: informationSchema,
  provider: providerSchema,
} as const
