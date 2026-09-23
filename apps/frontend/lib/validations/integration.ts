import { z } from 'zod'

export const createGitConnectionSchema = z.object({
  provider: z.enum(['github', 'gitlab', 'bitbucket', 'generic']),
  accountLogin: z
    .string()
    .min(1, 'Account or organization username is required')
    .max(100, 'Account name must be under 100 characters')
    .trim(),
  displayName: z
    .string()
    .min(1, 'Display name is required')
    .max(100, 'Display name must be under 100 characters')
    .trim(),
  accessToken: z
    .string()
    .min(1, 'Personal access token or credentials are required')
    .trim(),
  webhookSecret: z
    .string()
    .max(256, 'Webhook secret must be under 256 characters')
    .optional(),
})

export type CreateGitConnectionValues = z.infer<typeof createGitConnectionSchema>

export const createRegistrySchema = z.object({
  name: z
    .string()
    .min(1, 'Registry name is required')
    .max(100, 'Name must be under 100 characters')
    .trim(),
  provider: z.enum(['ghcr', 'dockerhub', 'gcp', 'ecr', 'acr', 'oci']),
  registryUrl: z
    .string()
    .min(1, 'Registry URL is required')
    .trim(),
  username: z
    .string()
    .min(1, 'Username or access key is required')
    .max(100)
    .trim(),
  token: z
    .string()
    .min(1, 'Password or access token is required')
    .trim(),
})

export type CreateRegistryValues = z.infer<typeof createRegistrySchema>

export const createNotificationChannelSchema = z.object({
  name: z
    .string()
    .min(1, 'Channel name is required')
    .max(100, 'Channel name must be under 100 characters')
    .trim(),
  type: z.enum(['EMAIL', 'SLACK', 'TEAMS', 'DISCORD', 'TELEGRAM', 'WEBHOOK', 'WHATSAPP']),
  target: z
    .string()
    .min(1, 'Destination (email address, webhook URL, or identifier) is required')
    .trim(),
  credential: z
    .string()
    .optional(),
  enabled: z.boolean(),
})

export type CreateNotificationChannelValues = z.infer<typeof createNotificationChannelSchema>

export const createNotificationPolicySchema = z.object({
  name: z
    .string()
    .min(1, 'Policy name is required')
    .max(100, 'Policy name must be under 100 characters')
    .trim(),
  eventTypes: z
    .array(z.string())
    .min(1, 'Select at least one event type'),
  channelIds: z
    .array(z.string())
    .min(1, 'Select at least one notification channel'),
  enabled: z.boolean(),
})

export type CreateNotificationPolicyValues = z.infer<typeof createNotificationPolicySchema>

export const createWebhookSchema = z.object({
  name: z
    .string()
    .min(1, 'Webhook name is required')
    .max(100, 'Webhook name must be under 100 characters')
    .trim(),
  url: z
    .string()
    .url('Must be a valid HTTP or HTTPS URL')
    .trim(),
  events: z
    .array(z.string())
    .min(1, 'Select at least one event subscription'),
  secret: z
    .string()
    .optional(),
  failureThreshold: z
    .number()
    .int()
    .min(1)
    .max(50),
  enabled: z.boolean(),
})

export type CreateWebhookValues = z.infer<typeof createWebhookSchema>
