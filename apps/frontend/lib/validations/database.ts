import { z } from 'zod'

export const DATABASE_ENGINES = ['PostgreSQL', 'MySQL', 'Redis', 'MongoDB'] as const
export type DatabaseEngine = (typeof DATABASE_ENGINES)[number]

export const POSTGRES_VERSIONS = ['16', '15', '14'] as const

export const createDatabaseSchema = z.object({
  name: z
    .string()
    .trim()
    .min(2, 'Name must be at least 2 characters')
    .max(63, 'Name must be 63 characters or fewer')
    .regex(/^[a-z0-9][a-z0-9-]*[a-z0-9]$/, 'Must be lowercase alphanumeric with hyphens'),
  type: z.enum(DATABASE_ENGINES),
  version: z.string().min(1, 'Engine version is required'),
  project: z.string().trim().min(1, 'Project is required'),
  environment: z.string().trim().min(1, 'Environment is required'),
  server: z.string().trim().min(1, 'Server is required'),
  dbName: z
    .string()
    .trim()
    .min(1, 'Database name is required')
    .regex(/^[a-zA-Z0-9_]+$/, 'Database name can only contain letters, numbers, and underscores'),
  username: z
    .string()
    .trim()
    .min(1, 'Username is required')
    .regex(/^[a-zA-Z0-9_]+$/, 'Username can only contain letters, numbers, and underscores'),
  storageTotalGb: z
    .number()
    .int('Disk size must be an integer')
    .min(5, 'Minimum allocated disk size is 5 GB')
    .max(10000, 'Maximum allocated disk size is 10,000 GB'),
  credentialsRevealAllowed: z.boolean(),
})

export type CreateDatabaseFormValues = z.infer<typeof createDatabaseSchema>

export const DEFAULT_CREATE_DATABASE_VALUES: CreateDatabaseFormValues = {
  name: '',
  type: 'PostgreSQL',
  version: '16',
  project: 'Daya Platform',
  environment: 'Production',
  server: 'prod-edge-01',
  dbName: 'main_db',
  username: 'postgres',
  storageTotalGb: 20,
  credentialsRevealAllowed: true,
}

export const restoreDatabaseSchema = z.object({
  backupRunId: z.string().min(1, 'Select a backup run to restore'),
  // Must match apps/api RestoreConfirmPhrase — do not weaken to database name.
  confirm: z
    .string()
    .trim()
    .refine((v) => v === 'RESTORE', {
      message: 'Type RESTORE to confirm destructive overwrite',
    }),
})

export type RestoreDatabaseFormValues = z.infer<typeof restoreDatabaseSchema>

export const backupNowSchema = z.object({
  destination: z.string().trim().min(1, 'Destination URI is required'),
  retentionDays: z
    .number()
    .int()
    .min(1, 'Retention must be at least 1 day')
    .max(3650, 'Retention cannot exceed 10 years'),
})

export type BackupNowFormValues = z.infer<typeof backupNowSchema>
