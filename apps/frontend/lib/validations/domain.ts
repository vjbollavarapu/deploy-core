import { z } from 'zod'

export const addDomainSchema = z.object({
  domain: z
    .string()
    .min(3, 'Domain must be at least 3 characters')
    .max(253, 'Domain too long')
    .regex(
      /^([a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,}$/i,
      'Must be a valid FQDN (e.g. api.example.com)',
    ),
  applicationId: z.string().min(1, 'Select an application'),
  routingPort: z.number().int().min(1, 'Port must be >= 1').max(65535, 'Port must be <= 65535'),
  forceHttps: z.boolean(),
  isPrimary: z.boolean(),
})

export type AddDomainValues = z.infer<typeof addDomainSchema>

export const DEFAULT_ADD_DOMAIN_VALUES: AddDomainValues = {
  domain: '',
  applicationId: '',
  routingPort: 3000,
  forceHttps: true,
  isPrimary: false,
}
