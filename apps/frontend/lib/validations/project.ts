import { z } from 'zod'

export const projectFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(2, 'Name must be at least 2 characters')
    .max(64, 'Name must be at most 64 characters'),
  slug: z
    .string()
    .trim()
    .min(2, 'Slug must be at least 2 characters')
    .max(64, 'Slug must be at most 64 characters')
    .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, 'Use lowercase letters, numbers, and hyphens'),
  description: z.string().trim().max(240, 'Description must be at most 240 characters').optional(),
})

export type ProjectFormValues = z.infer<typeof projectFormSchema>

export const environmentFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(2, 'Name must be at least 2 characters')
    .max(32, 'Name must be at most 32 characters'),
  slug: z
    .string()
    .trim()
    .min(2, 'Slug must be at least 2 characters')
    .max(32, 'Slug must be at most 32 characters')
    .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, 'Use lowercase letters, numbers, and hyphens'),
  type: z.enum(['Production', 'Staging', 'Preview', 'Development']),
})

export type EnvironmentFormValues = z.infer<typeof environmentFormSchema>

export function slugify(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
}
