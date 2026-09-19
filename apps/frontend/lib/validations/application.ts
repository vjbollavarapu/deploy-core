import { z } from 'zod'

export const SOURCE_TYPES = ['git', 'docker-image', 'docker-compose'] as const
export type SourceType = (typeof SOURCE_TYPES)[number]

export const APPLICATION_TYPES = [
  'Web Service',
  'API',
  'Worker',
  'Scheduled Job',
  'Static Site',
  'Docker Compose',
  'Docker Image',
] as const

export const RESTART_POLICIES = ['always', 'on-failure', 'unless-stopped', 'no'] as const

export const sourceStepSchema = z.object({
  sourceType: z.enum(SOURCE_TYPES, { message: 'Select a source type' }),
})

export const sourceConfigStepSchema = z
  .object({
    sourceType: z.enum(SOURCE_TYPES),
    repository: z.string().trim(),
    branch: z.string().trim(),
    dockerfile: z.string().trim(),
    buildContext: z.string().trim(),
    image: z.string().trim(),
    imageTag: z.string().trim(),
    composeFile: z.string().trim(),
  })
  .superRefine((values, ctx) => {
    if (values.sourceType === 'git') {
      if (values.repository.length < 3) {
        ctx.addIssue({ code: 'custom', path: ['repository'], message: 'Repository is required' })
      }
      if (values.branch.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['branch'], message: 'Branch is required' })
      }
      if (values.dockerfile.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['dockerfile'], message: 'Dockerfile path is required' })
      }
      if (values.buildContext.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['buildContext'], message: 'Build context is required' })
      }
    }
    if (values.sourceType === 'docker-image') {
      if (values.image.length < 2) {
        ctx.addIssue({ code: 'custom', path: ['image'], message: 'Image is required' })
      }
      if (values.imageTag.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['imageTag'], message: 'Tag is required' })
      }
    }
    if (values.sourceType === 'docker-compose') {
      if (values.composeFile.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['composeFile'], message: 'Compose file path is required' })
      }
      if (values.repository.length < 3) {
        ctx.addIssue({
          code: 'custom',
          path: ['repository'],
          message: 'Repository containing the compose file is required',
        })
      }
      if (values.branch.length < 1) {
        ctx.addIssue({ code: 'custom', path: ['branch'], message: 'Branch is required' })
      }
    }
  })

export const runtimeStepSchema = z.object({
  applicationType: z.enum(APPLICATION_TYPES, { message: 'Select an application type' }),
  port: z.coerce.number().int().min(1, 'Port must be at least 1').max(65535, 'Port must be at most 65535'),
  command: z.string().trim().max(512).optional().or(z.literal('')),
  entrypoint: z.string().trim().max(512).optional().or(z.literal('')),
  cpu: z.coerce.number().min(0.1, 'CPU must be at least 0.1').max(64, 'CPU must be at most 64'),
  memoryMb: z.coerce.number().int().min(64, 'RAM must be at least 64 MiB').max(262144, 'RAM is too large'),
  restartPolicy: z.enum(RESTART_POLICIES, { message: 'Select a restart policy' }),
})

const envVarSchema = z.object({
  key: z
    .string()
    .trim()
    .min(1, 'Key is required')
    .regex(/^[A-Z][A-Z0-9_]*$/, 'Use UPPER_SNAKE_CASE'),
  value: z.string(),
})

const secretRefSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Secret name is required')
    .regex(/^[A-Z][A-Z0-9_]*$/, 'Use UPPER_SNAKE_CASE'),
})

export const configurationStepSchema = z.object({
  envVars: z.array(envVarSchema).max(50, 'Too many environment variables'),
  secrets: z.array(secretRefSchema).max(50, 'Too many secrets'),
})

export const networkingStepSchema = z.object({
  domain: z
    .string()
    .trim()
    .optional()
    .or(z.literal(''))
    .refine((value) => !value || /^[a-z0-9.-]+\.[a-z]{2,}$/i.test(value), 'Enter a valid domain'),
  healthCheckPath: z.string().trim().min(1, 'Health check path is required'),
  healthCheckPort: z.coerce
    .number()
    .int()
    .min(1, 'Port must be at least 1')
    .max(65535, 'Port must be at most 65535'),
})

export const placementStepSchema = z.object({
  name: z
    .string()
    .trim()
    .min(2, 'Name must be at least 2 characters')
    .max(64, 'Name must be at most 64 characters')
    .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, 'Use lowercase letters, numbers, and hyphens'),
  projectId: z.string().min(1, 'Select a project'),
  environment: z.string().min(1, 'Select an environment'),
  serverId: z.string().min(1, 'Select a target server'),
})

export const createApplicationSchema = z
  .object({
    sourceType: z.enum(SOURCE_TYPES),
    repository: z.string(),
    branch: z.string(),
    dockerfile: z.string(),
    buildContext: z.string(),
    image: z.string(),
    imageTag: z.string(),
    composeFile: z.string(),
    applicationType: z.enum(APPLICATION_TYPES),
    port: z.coerce.number().int().min(1).max(65535),
    command: z.string().optional().or(z.literal('')),
    entrypoint: z.string().optional().or(z.literal('')),
    cpu: z.coerce.number().min(0.1).max(64),
    memoryMb: z.coerce.number().int().min(64).max(262144),
    restartPolicy: z.enum(RESTART_POLICIES),
    envVars: z.array(envVarSchema).max(50),
    secrets: z.array(secretRefSchema).max(50),
    domain: z
      .string()
      .optional()
      .or(z.literal(''))
      .refine((value) => !value || /^[a-z0-9.-]+\.[a-z]{2,}$/i.test(value), 'Enter a valid domain'),
    healthCheckPath: z.string().min(1),
    healthCheckPort: z.coerce.number().int().min(1).max(65535),
    name: z
      .string()
      .trim()
      .min(2)
      .max(64)
      .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/),
    projectId: z.string().min(1),
    environment: z.string().min(1),
    serverId: z.string().min(1),
  })
  .superRefine((values, ctx) => {
    const sourceResult = sourceConfigStepSchema.safeParse(values)
    if (!sourceResult.success) {
      for (const issue of sourceResult.error.issues) {
        ctx.addIssue({
          code: 'custom',
          path: issue.path,
          message: issue.message,
        })
      }
    }
  })

export type CreateApplicationValues = z.infer<typeof createApplicationSchema>

export const WIZARD_STEPS = [
  { id: 'source', label: 'Source', schema: sourceStepSchema },
  { id: 'source-config', label: 'Source config', schema: sourceConfigStepSchema },
  { id: 'runtime', label: 'Runtime', schema: runtimeStepSchema },
  { id: 'configuration', label: 'Configuration', schema: configurationStepSchema },
  { id: 'networking', label: 'Networking', schema: networkingStepSchema },
  { id: 'placement', label: 'Placement', schema: placementStepSchema },
  { id: 'review', label: 'Review' },
  { id: 'deploy', label: 'Deploy' },
] as const

export const DEFAULT_APPLICATION_VALUES: CreateApplicationValues = {
  sourceType: 'git',
  repository: '',
  branch: 'main',
  dockerfile: 'Dockerfile',
  buildContext: '.',
  image: '',
  imageTag: 'latest',
  composeFile: 'docker-compose.yml',
  applicationType: 'Web Service',
  port: 8080,
  command: '',
  entrypoint: '',
  cpu: 0.5,
  memoryMb: 512,
  restartPolicy: 'unless-stopped',
  envVars: [],
  secrets: [],
  domain: '',
  healthCheckPath: '/healthz',
  healthCheckPort: 8080,
  name: '',
  projectId: '',
  environment: '',
  serverId: '',
}

export function sourceTypeLabel(type: SourceType): string {
  switch (type) {
    case 'git':
      return 'Git Repository'
    case 'docker-image':
      return 'Docker Image'
    case 'docker-compose':
      return 'Docker Compose'
  }
}
